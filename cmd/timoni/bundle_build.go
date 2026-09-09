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
	"io"
	"maps"
	"os"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"sort"
	"strings"
	"sync"

	"cuelang.org/go/cue/cuecontext"
	"github.com/fluxcd/pkg/ssa"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/mask"
	"github.com/stefanprodan/timoni/internal/runtime"
)

var bundleBuildCmd = &cobra.Command{
	Use:     "build",
	Aliases: []string{"template"},
	Short:   "Build and print the resulting Kubernetes resources for all instances from a Bundle",
	Long: `The bundle build command builds and prints the resulting Kubernetes resources for all instances defined in a Bundle.

Custom resources are validated against the CRD schemas vendored in the modules and the CRDs rendered by any instance of the bundle. A violation fails the command and nothing is written. Use --validate=false to disable the validation.

With --update, module references are updated, vetted, and built from the same in-memory inputs. Bundle files are written only after vet and build succeed. Use --local-index for extracted local modules and repeatable --oci mappings for local OCI artifacts.
`,
	Example: `  # Build all instances from a bundle and print the manifests to stdout
  timoni bundle build -f bundle.cue

  # Pass secret values from stdin
  cat ./bundle_secrets.cue | timoni bundle build -f ./bundle.cue -f -

  # Write the manifests as a directory tree, one directory per instance
  # and one file per resource, named like 'kustomize build -o <dir>'
  timoni bundle build -f bundle.cue --output-dir ./manifests

  # Update an indexed local module and render the updated bundle
  timoni bundle build --update --local-index module-index.cue -f bundle.cue --runtime-from-env
`,
	Args: cobra.NoArgs,
	RunE: runBundleBuildCmd,
}

type bundleBuildFlags struct {
	pkg         flags.Package
	files       []string
	creds       flags.Credentials
	update      bool
	level       string
	localIndex  string
	localOCI    []string
	outputDir   string
	concurrency int
	maskSecrets bool
	validate    bool
}

var bundleBuildArgs bundleBuildFlags

func init() {
	bundleBuildCmd.Flags().VarP(&bundleBuildArgs.pkg, bundleBuildArgs.pkg.Type(), bundleBuildArgs.pkg.Shorthand(), bundleBuildArgs.pkg.Description())
	bundleBuildCmd.Flags().StringSliceVarP(&bundleBuildArgs.files, "file", "f", nil,
		"The local path to bundle.cue files.")
	bundleBuildCmd.Flags().Var(&bundleBuildArgs.creds, bundleBuildArgs.creds.Type(), bundleBuildArgs.creds.Description())
	bundleBuildCmd.Flags().BoolVar(&bundleBuildArgs.update, "update", false,
		"Update module references before vetting and building the bundle.")
	bundleBuildCmd.Flags().StringVar(&bundleBuildArgs.level, "level", engine.UpdateLevelNone,
		"Set the update level for module references without an update attribute: none, patch, minor, or major; requires --update.")
	bundleBuildCmd.Flags().StringVar(&bundleBuildArgs.localIndex, "local-index", "",
		"CUE file that maps module identities and semantic versions to verified local sources; requires --update.")
	bundleBuildCmd.Flags().StringArrayVar(&bundleBuildArgs.localOCI, "oci", nil,
		"Map a local OCI archive or image layout to a repository as 'oci://repository=path'; repeatable; requires --update.")
	bundleBuildCmd.Flags().StringVar(&bundleBuildArgs.outputDir, "output-dir", "",
		"The path to a directory where the manifests are written as a tree, one directory per instance and one file per resource.")
	bundleBuildCmd.Flags().IntVar(&bundleBuildArgs.concurrency, "concurrency", 0,
		"The number of instances to build concurrently, defaults to the number of CPU cores capped at 8.")
	bundleBuildCmd.Flags().BoolVar(&bundleBuildArgs.maskSecrets, "mask-secrets", false,
		"Hide the values of Kubernetes Secrets in the printed objects, ignored with --output-dir.")
	bundleBuildCmd.Flags().BoolVar(&bundleBuildArgs.validate, "validate", true,
		"Validate the custom resources against their CRD schemas and CEL rules.")
	bundleCmd.AddCommand(bundleBuildCmd)
}

func runBundleBuildCmd(cmd *cobra.Command, _ []string) error {
	files := slices.Clone(bundleBuildArgs.files)
	if len(files) == 0 {
		return errors.New("no bundle provided with -f")
	}
	if bundleBuildArgs.update && slices.Contains(files, "-") {
		return errors.New("--update cannot be used with -f -")
	}
	if !bundleBuildArgs.update && (cmd.Flags().Changed("level") || cmd.Flags().Changed("local-index") || cmd.Flags().Changed("oci")) {
		return errors.New("--update is required with --level, --local-index, and --oci")
	}
	if bundleBuildArgs.update && bundleBuildArgs.outputDir != "" {
		return errors.New("--update cannot be used with --output-dir")
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

	workdir, err := resolveWorkdir(bundleArgs.workdir)
	if err != nil {
		return err
	}
	if bundleBuildArgs.update {
		return runBundleBuildWithUpdate(cmd, files, workdir)
	}
	return runBundleBuild(cmd, files, workdir, nil)
}

func runBundleBuild(cmd *cobra.Command, files []string, workdir string, overrides map[string][]byte) error {
	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctx := cuecontext.New()
	bm := engine.NewBundleBuilder(ctx, files)
	bm.SetWorkdir(workdir)
	bm.SetFileOverrides(overrides)

	workspace, runtimeValues, err := resolveBundleBuildRuntime(cmd)
	if err != nil {
		return err
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

	// Build the instances concurrently, each in its own CUE context, and
	// retain the objects in the order defined by the bundle.
	objectsByInstance := make([][]*unstructured.Unstructured, len(bundle.Instances))
	var vendoredCache *vendoredCRDCache
	if bundleBuildArgs.validate {
		vendoredCache = &vendoredCRDCache{crds: make(map[string][]*unstructured.Unstructured)}
	}
	eg := errgroup.Group{}
	eg.SetLimit(buildConcurrency())
	for i, instance := range bundle.Instances {
		eg.Go(func() error {
			objects, err := buildBundleInstanceObjects(instance, modDirs[instance.Name], vendoredCache)
			if err != nil {
				return err
			}
			objectsByInstance[i] = objects
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	if bundleBuildArgs.validate {
		if err := validateBundleInstanceObjects(cmd, bundle.Instances, modDirs, objectsByInstance, vendoredCache); err != nil {
			return err
		}
	}

	if bundleBuildArgs.outputDir != "" {
		return writeBundleInstancesToDir(cmd, bundle.Instances, objectsByInstance)
	}

	return printBundleInstances(cmd, bundle.Instances, objectsByInstance)
}

// resolveBundleBuildRuntime returns the workspace name and the runtime values
// for the bundle build: the process environment with --runtime-from-env, and
// the values read from the single cluster selected by --runtime.
func resolveBundleBuildRuntime(cmd *cobra.Command) (string, map[string]string, error) {
	workspace := apiv1.RuntimeDefaultName
	runtimeValues := make(map[string]string)

	if bundleArgs.runtimeFromEnv {
		maps.Copy(runtimeValues, engine.GetEnv())
	}

	if len(bundleArgs.runtimeFiles) == 0 {
		return workspace, runtimeValues, nil
	}

	kctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	rt, err := buildRuntime(bundleArgs.runtimeFiles, bundleArgs.workdir)
	if err != nil {
		return "", nil, err
	}

	clusters := rt.SelectClusters(bundleArgs.runtimeCluster, bundleArgs.runtimeClusterGroup)
	if len(clusters) > 1 {
		return "", nil, errors.New("you must select a cluster with --runtime-cluster")
	}
	if len(clusters) == 0 {
		return "", nil, errors.New("no cluster found")
	}

	cluster := clusters[0]
	workspace = cluster.Name
	kubeconfigArgs.Context = &cluster.KubeContext

	rm, err := runtime.NewResourceManager(kubeconfigArgs)
	if err != nil {
		return "", nil, err
	}

	reader := runtime.NewResourceReader(rm)
	rv, err := reader.Read(kctx, rt.Refs)
	if err != nil {
		return "", nil, err
	}

	maps.Copy(runtimeValues, rv)
	maps.Copy(runtimeValues, cluster.NameGroupValues())

	return workspace, runtimeValues, nil
}

// validateBundleInstanceObjects validates the custom resources of all the
// instances against the vendored CRDs, registered once per module, and the
// CRDs rendered by the instances in bundle order; the rendered CRDs take
// precedence for the same kind versions.
func validateBundleInstanceObjects(cmd *cobra.Command, instances []*apiv1.BundleInstance, modDirs map[string]string,
	objectsByInstance [][]*unstructured.Unstructured, vendoredCache *vendoredCRDCache) error {
	crdValidator := engine.NewCRDValidator()
	registered := make(map[string]bool)
	for _, instance := range instances {
		modDir := modDirs[instance.Name]
		if registered[modDir] {
			continue
		}
		registered[modDir] = true
		vendoredCRDs, _ := vendoredCache.get(modDir)
		if err := crdValidator.AddVendoredCRDs(vendoredCRDs); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}
	for _, objects := range objectsByInstance {
		if err := crdValidator.AddCRDs(objects); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}

	invalid := 0
	log := LoggerFrom(cmd.Context())
	for i, objects := range objectsByInstance {
		validationErrs := crdValidator.ValidateObjects(cmd.Context(), objects)
		invalid += logValidationErrors(log, validationErrs, instances[i].Name)
	}
	if invalid > 0 {
		return fmt.Errorf("validation failed, %d invalid custom resource(s)", invalid)
	}
	return nil
}

// printBundleInstances marshals the objects of each instance to YAML
// concurrently, masking the Secret values with --mask-secrets, and writes
// the manifests to the command output in the order defined by the bundle.
func printBundleInstances(cmd *cobra.Command, instances []*apiv1.BundleInstance, objectsByInstance [][]*unstructured.Unstructured) error {
	manifests := make([]string, len(instances))
	eg := errgroup.Group{}
	eg.SetLimit(buildConcurrency())
	for i := range instances {
		eg.Go(func() error {
			objects := objectsByInstance[i]
			if bundleBuildArgs.maskSecrets {
				for j, obj := range objects {
					objects[j] = mask.SecretData(obj)
				}
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
	for i, instance := range instances {
		sb.WriteString("---\n")
		sb.WriteString(fmt.Sprintf("# Instance: %s\n", instance.Name))
		sb.WriteString("---\n")
		sb.WriteString(manifests[i])
		if i < len(instances)-1 {
			sb.WriteString("\n")
		}
	}

	data := []byte(sb.String())
	n, err := cmd.OutOrStdout().Write(data)
	if err != nil {
		return err
	}
	if n < len(data) {
		return io.ErrShortWrite
	}

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

// writeBundleInstancesToDir writes previously built resources to the output
// directory as a tree: one directory per instance and one file per resource,
// named with the same convention as 'kustomize build -o <dir>'.
func writeBundleInstancesToDir(cmd *cobra.Command, instances []*apiv1.BundleInstance, objectsByInstance [][]*unstructured.Unstructured) error {
	outputDir := bundleBuildArgs.outputDir
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write each instance to its own directory concurrently, then report the
	// results in the order defined by the bundle.
	exported := make([]string, len(instances))
	eg := errgroup.Group{}
	eg.SetLimit(buildConcurrency())
	for i, instance := range instances {
		eg.Go(func() error {
			objects := objectsByInstance[i]
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

// vendoredCRDCache stores extracted vendored CRDs by module directory.
type vendoredCRDCache struct {
	mu   sync.Mutex
	crds map[string][]*unstructured.Unstructured
}

// get returns the cached vendored CRDs for a module directory.
func (c *vendoredCRDCache) get(modDir string) ([]*unstructured.Unstructured, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	crds, ok := c.crds[modDir]
	return crds, ok
}

// put caches vendored CRDs for a module directory.
func (c *vendoredCRDCache) put(modDir string, crds []*unstructured.Unstructured) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.crds[modDir] = crds
}

// buildBundleInstanceObjects builds an instance and returns its sorted
// Kubernetes objects. The instance is compiled in its own CUE context so
// that the memory used during the build can be reclaimed once the objects
// are extracted, leaving only the rendered objects of the built instances
// in memory until they are written. The module directory is shared between
// the instances referencing the same module version and is never modified;
// the instance schema and values are injected as in-memory overlays.
// When a cache is given, the CRDs vendored in the module are extracted
// while the CUE context is alive and stored in the cache under the module
// directory, unless another instance of the same module stored them already.
func buildBundleInstanceObjects(instance *apiv1.BundleInstance, modDir string, cache *vendoredCRDCache) ([]*unstructured.Unstructured, error) {
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

	if cache != nil {
		if _, found := cache.get(modDir); !found {
			vendoredCRDs, err := builder.GetVendoredCRDs()
			if err != nil {
				return nil, fmt.Errorf("failed to extract vendored CRDs: %w", err)
			}
			cache.put(modDir, vendoredCRDs)
		}
	}

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
