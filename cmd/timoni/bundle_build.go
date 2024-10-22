/*
Copyright 2023 Stefan Prodan

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/fluxcd/pkg/ssa"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/runtime"
)

var bundleBuildCmd = &cobra.Command{
	Use:     "build",
	Aliases: []string{"template"},
	Short:   "Build and print the resulting Kubernetes resources for all instances from a Bundle",
	Long: `The bundle build command builds and prints the resulting Kubernetes resources for all instances defined in a Bundle.
`,
	Example: `  # Build all instances from a bundle
  timoni bundle build -f bundle.cue

  # Pass secret values from stdin
  cat ./bundle_secrets.cue | timoni bundle build -f ./bundle.cue -f -
`,
	Args: cobra.NoArgs,
	RunE: runBundleBuildCmd,
}

type bundleBuildFlags struct {
	pkg   flags.Package
	files []string
	creds flags.Credentials
}

var bundleBuildArgs bundleBuildFlags

func init() {
	bundleBuildCmd.Flags().VarP(&bundleBuildArgs.pkg, bundleBuildArgs.pkg.Type(), bundleBuildArgs.pkg.Shorthand(), bundleBuildArgs.pkg.Description())
	bundleBuildCmd.Flags().StringSliceVarP(&bundleBuildArgs.files, "file", "f", nil,
		"The local path to bundle.cue files.")
	bundleBuildCmd.Flags().Var(&bundleBuildArgs.creds, bundleBuildArgs.creds.Type(), bundleBuildArgs.creds.Description())
	bundleCmd.AddCommand(bundleBuildCmd)
}

func runBundleBuildCmd(cmd *cobra.Command, _ []string) error {
	files := bundleBuildArgs.files
	if len(files) == 0 {
		return errors.New("no bundle provided with -f")
	}
	var stdinFile string
	for i, file := range files {
		if file == "-" {
			stdinFile, err := saveReaderToFile(cmd.InOrStdin())
			if err != nil {
				return err
			}
			files[i] = stdinFile
			break
		}
	}
	if stdinFile != "" {
		defer os.Remove(stdinFile)
	}

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctx := cuecontext.New()
	bm := engine.NewBundleBuilder(ctx, files)

	runtimeValues := make(map[string]string)

	if bundleArgs.runtimeFromEnv {
		maps.Copy(runtimeValues, engine.GetEnv())
	}

	if len(bundleArgs.runtimeFiles) > 0 {
		kctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
		defer cancel()

		rt, err := buildRuntime(bundleArgs.runtimeFiles)
		if err != nil {
			return err
		}

		clusters := rt.SelectClusters(bundleArgs.runtimeCluster, bundleArgs.runtimeClusterGroup)
		if len(clusters) > 1 {
			return errors.New("you must select a cluster with --runtime-cluster")
		}
		if len(clusters) == 0 {
			return errors.New("no cluster found")
		}

		cluster := clusters[0]
		kubeconfigArgs.Context = &cluster.KubeContext

		rm, err := runtime.NewResourceManager(kubeconfigArgs)
		if err != nil {
			return err
		}

		reader := runtime.NewResourceReader(rm)
		rv, err := reader.Read(kctx, rt.Refs)
		if err != nil {
			return err
		}

		maps.Copy(runtimeValues, rv)
		maps.Copy(runtimeValues, cluster.NameGroupValues())
	}

	if err := bm.InitWorkspace(tmpDir, runtimeValues); err != nil {
		return describeErr(tmpDir, "failed to parse bundle", err)
	}

	v, err := bm.Build(tmpDir)
	if err != nil {
		return describeErr(tmpDir, "failed to build bundle", err)
	}

	bundle, err := bm.GetBundle(v)
	if err != nil {
		return err
	}

	var sb strings.Builder

	ctxPull, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	for _, instance := range bundle.Instances {
		if err := fetchBundleInstanceModule(ctxPull, instance, tmpDir); err != nil {
			return err
		}
	}

	for i, instance := range bundle.Instances {
		sb.WriteString("---\n")
		sb.WriteString(fmt.Sprintf("# Instance: %s\n", instance.Name))
		sb.WriteString("---\n")

		instance, err := buildBundleInstance(ctx, instance, tmpDir)
		if err != nil {
			return err
		}

		sb.WriteString(instance)
		if i < len(bundle.Instances)-1 {
			sb.WriteString("\n")
		}
	}

	cmd.OutOrStdout().Write([]byte(sb.String()))

	return nil
}

func buildBundleInstance(cuectx *cue.Context, instance *engine.BundleInstance, rootDir string) (string, error) {
	modDir := path.Join(rootDir, instance.Name, "module")

	builder := engine.NewModuleBuilder(
		cuectx,
		instance.Name,
		instance.Namespace,
		modDir,
		bundleBuildArgs.pkg.String(),
	)

	if err := builder.WriteSchemaFile(); err != nil {
		return "", err
	}

	modName, err := builder.GetModuleName()
	if err != nil {
		return "", err
	}
	instance.Module.Name = modName

	err = builder.WriteValuesFileWithDefaults(instance.Values)
	if err != nil {
		return "", err
	}

	builder.SetVersionInfo(instance.Module.Version, "")

	buildResult, err := builder.Build()
	if err != nil {
		return "", describeErr(modDir, "build failed for "+instance.Name, err)
	}

	bundleBuildSets, err := builder.GetApplySets(buildResult)
	if err != nil {
		return "", fmt.Errorf("failed to extract objects: %w", err)
	}

	var objects []*unstructured.Unstructured
	for _, set := range bundleBuildSets {
		objects = append(objects, set.Objects...)
	}
	sort.Sort(ssa.SortableUnstructureds(objects))

	var sb strings.Builder

	for i, r := range objects {
		data, err := yaml.Marshal(r)
		if err != nil {
			return "", fmt.Errorf("converting objects failed: %w", err)
		}

		if i != 0 {
			sb.WriteString("---\n")
		}
		sb.Write(data)
	}

	return sb.String(), nil
}
