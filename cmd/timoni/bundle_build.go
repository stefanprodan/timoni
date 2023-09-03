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
	"fmt"
	"os"
	"sort"
	"strings"

	"cuelang.org/go/cue/cuecontext"
	"github.com/fluxcd/pkg/ssa"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/flags"
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
	RunE: runBundleBuildCmd,
}

type bundleBuildFlags struct {
	pkg            flags.Package
	files          []string
	creds          flags.Credentials
	runtimeFromEnv bool
}

var bundleBuildArgs bundleBuildFlags

func init() {
	bundleBuildCmd.Flags().VarP(&bundleBuildArgs.pkg, bundleBuildArgs.pkg.Type(), bundleBuildArgs.pkg.Shorthand(), bundleBuildArgs.pkg.Description())
	bundleBuildCmd.Flags().StringSliceVarP(&bundleBuildArgs.files, "file", "f", nil,
		"The local path to bundle.cue files.")
	bundleBuildCmd.Flags().BoolVar(&bundleBuildArgs.runtimeFromEnv, "runtime-from-env", false,
		"Inject runtime values from the environment.")
	bundleBuildCmd.Flags().Var(&bundleBuildArgs.creds, bundleBuildArgs.creds.Type(), bundleBuildArgs.creds.Description())
	bundleCmd.AddCommand(bundleBuildCmd)
}

func runBundleBuildCmd(cmd *cobra.Command, _ []string) error {
	files := bundleBuildArgs.files
	for i, file := range files {
		if file == "-" {
			path, err := saveReaderToFile(cmd.InOrStdin())
			if err != nil {
				return err
			}

			defer os.Remove(path)

			files[i] = path
		}
	}

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctx := cuecontext.New()
	bm := engine.NewBundleBuilder(ctx, files)

	runtimeValues := make(map[string]string)

	if bundleBuildArgs.runtimeFromEnv {
		runtimeValues = engine.GetEnv()
	}

	if err := bm.InitWorkspace(tmpDir, runtimeValues); err != nil {
		return describeErr(tmpDir, "failed to parse bundle", err)
	}

	v, err := bm.Build()
	if err != nil {
		return describeErr(tmpDir, "failed to build bundle", err)
	}

	bundle, err := bm.GetBundle(v)
	if err != nil {
		return err
	}

	var sb strings.Builder

	for i, instance := range bundle.Instances {
		sb.WriteString("---\n")
		sb.WriteString(fmt.Sprintf("# Instance: %s\n", instance.Name))
		sb.WriteString("---\n")

		instance, err := buildBundleInstance(instance)
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

func buildBundleInstance(instance engine.BundleInstance) (string, error) {
	moduleVersion := instance.Module.Version

	if moduleVersion == engine.LatestTag && instance.Module.Digest != "" {
		moduleVersion = "@" + instance.Module.Digest
	}

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	ctxPull, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	fetcher := engine.NewFetcher(
		ctxPull,
		instance.Module.Repository,
		moduleVersion,
		tmpDir,
		bundleBuildArgs.creds.String(),
	)
	mod, err := fetcher.Fetch()
	if err != nil {
		return "", err
	}

	if instance.Module.Digest != "" && mod.Digest != instance.Module.Digest {
		return "", fmt.Errorf("the upstream digest %s of version %s doesn't match the specified digest %s",
			mod.Digest, instance.Module.Version, instance.Module.Digest)
	}

	cuectx := cuecontext.New()
	builder := engine.NewModuleBuilder(
		cuectx,
		instance.Name,
		instance.Namespace,
		fetcher.GetModuleRoot(),
		bundleBuildArgs.pkg.String(),
	)

	if err := builder.WriteSchemaFile(); err != nil {
		return "", err
	}

	mod.Name, err = builder.GetModuleName()
	if err != nil {
		return "", err
	}

	err = builder.WriteValuesFileWithDefaults(instance.Values)
	if err != nil {
		return "", err
	}

	buildResult, err := builder.Build()
	if err != nil {
		return "", describeErr(fetcher.GetModuleRoot(), "failed to build instance", err)
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
