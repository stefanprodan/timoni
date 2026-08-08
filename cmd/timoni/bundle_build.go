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
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"

	"cuelang.org/go/cue/cuecontext"
	"github.com/fluxcd/pkg/ssa"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
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
	Example: `  # Build all instances from a bundle and print the manifests to stdout
  timoni bundle build -f bundle.cue

  # Pass secret values from stdin
  cat ./bundle_secrets.cue | timoni bundle build -f ./bundle.cue -f -

  # Write the manifests as a directory tree, one directory per instance
  # and one file per resource, named like 'kustomize build -o <dir>'
  timoni bundle build -f bundle.cue --output-dir ./manifests
`,
	Args: cobra.NoArgs,
	RunE: runBundleBuildCmd,
}

type bundleBuildFlags struct {
	pkg         flags.Package
	files       []string
	creds       flags.Credentials
	outputDir   string
	concurrency int
}

var bundleBuildArgs bundleBuildFlags

func init() {
	bundleBuildCmd.Flags().VarP(&bundleBuildArgs.pkg, bundleBuildArgs.pkg.Type(), bundleBuildArgs.pkg.Shorthand(), bundleBuildArgs.pkg.Description())
	bundleBuildCmd.Flags().StringSliceVarP(&bundleBuildArgs.files, "file", "f", nil,
		"The local path to bundle.cue files.")
	bundleBuildCmd.Flags().Var(&bundleBuildArgs.creds, bundleBuildArgs.creds.Type(), bundleBuildArgs.creds.Description())
	bundleBuildCmd.Flags().StringVar(&bundleBuildArgs.outputDir, "output-dir", "",
		"The path to a directory where the manifests are written as a tree, one directory per instance and one file per resource.")
	bundleBuildCmd.Flags().IntVar(&bundleBuildArgs.concurrency, "concurrency", 0,
		"The number of instances to build concurrently, defaults to the number of CPU cores capped at 8.")
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

	workdir, err := resolveWorkdir(bundleArgs.workdir)
	if err != nil {
		return err
	}

	ctx := cuecontext.New()
	bm := engine.NewBundleBuilder(ctx, files)
	bm.SetWorkdir(workdir)

	workspace := apiv1.RuntimeDefaultName
	runtimeValues := make(map[string]string)

	if bundleArgs.runtimeFromEnv {
		maps.Copy(runtimeValues, engine.GetEnv())
	}

	if len(bundleArgs.runtimeFiles) > 0 {
		kctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
		defer cancel()

		rt, err := buildRuntime(bundleArgs.runtimeFiles, bundleArgs.workdir)
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
		workspace = cluster.Name
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

	if err := bm.InitWorkspace(workspace, runtimeValues); err != nil {
		return describeErr(bm.WorkspaceDir(workspace), "failed to parse bundle", err)
	}

	v, err := bm.Build(workspace)
	if err != nil {
		return describeErr(bm.WorkspaceDir(workspace), "failed to build bundle", err)
	}

	bundle, err := bm.GetBundle(v)
	if err != nil {
		return err
	}

	ctxPull, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	moduleCache := make(map[moduleCacheKey]*fetchedModule)
	modDirs := make(map[string]string)
	for _, instance := range bundle.Instances {
		modDir, err := fetchBundleInstanceModule(ctxPull, instance, tmpDir, bundleBuildArgs.creds.String(), moduleCache)
		if err != nil {
			return err
		}
		modDirs[instance.Name] = modDir
	}

	if bundleBuildArgs.outputDir != "" {
		return writeBundleInstancesToDir(cmd, bundle.Instances, modDirs)
	}

	// Build the instances concurrently, each in its own CUE context,
	// and assemble the manifests in the order defined by the bundle.
	manifests := make([]string, len(bundle.Instances))
	eg := errgroup.Group{}
	eg.SetLimit(buildConcurrency())
	for i, instance := range bundle.Instances {
		eg.Go(func() error {
			objects, err := buildBundleInstanceObjects(instance, modDirs[instance.Name])
			if err != nil {
				return err
			}

			m, err := marshalObjectsToYAML(objects)
			if err != nil {
				return err
			}

			manifests[i] = m
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	var sb strings.Builder
	for i, instance := range bundle.Instances {
		sb.WriteString("---\n")
		sb.WriteString(fmt.Sprintf("# Instance: %s\n", instance.Name))
		sb.WriteString("---\n")
		sb.WriteString(manifests[i])
		if i < len(bundle.Instances)-1 {
			sb.WriteString("\n")
		}
	}

	cmd.OutOrStdout().Write([]byte(sb.String()))

	return nil
}

// buildConcurrency returns the number of instances to build concurrently,
// by default bounded to keep the peak memory of large bundles in check as
// every in-flight instance holds its own CUE evaluation context.
func buildConcurrency() int {
	if bundleBuildArgs.concurrency > 0 {
		return bundleBuildArgs.concurrency
	}
	return min(goruntime.NumCPU(), 8)
}

// writeBundleInstancesToDir writes the resources of each instance to the
// output directory as a tree: one directory per instance and one file per
// resource, named with the same convention as 'kustomize build -o <dir>'.
func writeBundleInstancesToDir(cmd *cobra.Command, instances []*apiv1.BundleInstance, modDirs map[string]string) error {
	outputDir := bundleBuildArgs.outputDir
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Build the instances and write their resources concurrently, each
	// instance in its own CUE context and its own directory, then report
	// the results in the order defined by the bundle.
	exported := make([]string, len(instances))
	eg := errgroup.Group{}
	eg.SetLimit(buildConcurrency())
	for i, instance := range instances {
		eg.Go(func() error {
			objects, err := buildBundleInstanceObjects(instance, modDirs[instance.Name])
			if err != nil {
				return err
			}

			instanceDir := filepath.Join(outputDir, instance.Name)
			if err := os.MkdirAll(instanceDir, 0o755); err != nil {
				return fmt.Errorf("failed to create instance directory: %w", err)
			}

			// Prefix the file names with the namespace only when the instance's
			// namespaced resources span more than one namespace, matching the
			// behaviour of kustomize.
			//
			// The resource scope is approximated by the presence of
			// metadata.namespace rather than the true cluster/namespaced scope
			// (kustomize resolves this from type info). A cluster-scoped object
			// that carries a stray namespace is therefore counted here and can
			// enable the prefix for the whole instance. This is acceptable for
			// Timoni's offline builds, where no REST mapper is available to
			// resolve resource scopes.
			namespaces := make(map[string]struct{})
			for _, obj := range objects {
				if ns := obj.GetNamespace(); ns != "" {
					namespaces[ns] = struct{}{}
				}
			}
			withNamespace := len(namespaces) > 1

			for _, obj := range objects {
				data, err := yaml.Marshal(obj)
				if err != nil {
					return fmt.Errorf("converting objects failed: %w", err)
				}

				fileName := resourceFileName(obj, withNamespace && obj.GetNamespace() != "")
				if err := os.WriteFile(filepath.Join(instanceDir, fileName), data, 0o644); err != nil {
					return fmt.Errorf("failed to write manifest: %w", err)
				}
			}

			exported[i] = fmt.Sprintf("exported %d resources to %s", len(objects), instanceDir)
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	log := LoggerFrom(cmd.Context())
	for _, msg := range exported {
		log.Info(msg)
	}

	return nil
}

// resourceFileName returns the manifest file name for an object using the
// same convention as 'kustomize build -o <dir>':
// '<group>_<version>_<kind>_<name>.yaml', lowercased, with empty GVK fields
// omitted and an optional '<namespace>_' prefix.
func resourceFileName(obj *unstructured.Unstructured, withNamespace bool) string {
	gvk := obj.GroupVersionKind()

	parts := make([]string, 0, 3)
	if gvk.Group != "" {
		parts = append(parts, gvk.Group)
	}
	if gvk.Version != "" {
		parts = append(parts, gvk.Version)
	}
	parts = append(parts, gvk.Kind)

	fileName := strings.ToLower(strings.Join(parts, "_")) + "_" + strings.ToLower(obj.GetName()) + ".yaml"
	if withNamespace {
		fileName = strings.ToLower(obj.GetNamespace()) + "_" + fileName
	}
	return fileName
}

// buildBundleInstanceObjects builds an instance and returns its sorted
// Kubernetes objects. The instance is compiled in its own CUE context so
// that the memory used during the build can be reclaimed once the objects
// are extracted, keeping the peak usage constant regardless of how many
// instances a bundle contains. The module directory is shared between the
// instances referencing the same module version and is never modified; the
// instance schema and values are injected as in-memory overlays.
func buildBundleInstanceObjects(instance *apiv1.BundleInstance, modDir string) ([]*unstructured.Unstructured, error) {
	builder := engine.NewModuleBuilder(
		nil,
		instance.Name,
		instance.Namespace,
		modDir,
		bundleBuildArgs.pkg.String(),
	)

	if err := builder.OverlaySchemaFile(); err != nil {
		return nil, err
	}

	modName, err := builder.GetModuleName()
	if err != nil {
		return nil, err
	}
	instance.Module.Name = modName

	if err := builder.OverlayValuesFileWithDefaults(instance.Values); err != nil {
		return nil, err
	}

	builder.SetVersionInfo(instance.Module.Version, "")

	buildResult, err := builder.Build()
	if err != nil {
		return nil, describeErr(modDir, "build failed for "+instance.Name, err)
	}

	bundleBuildSets, err := builder.GetApplySets(buildResult)
	if err != nil {
		return nil, fmt.Errorf("failed to extract objects: %w", err)
	}

	var objects []*unstructured.Unstructured
	for _, set := range bundleBuildSets {
		objects = append(objects, set.Objects...)
	}
	sort.Sort(ssa.SortableUnstructureds(objects))

	return objects, nil
}

// marshalObjectsToYAML marshals the objects to a multi-document YAML string.
func marshalObjectsToYAML(objects []*unstructured.Unstructured) (string, error) {
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
