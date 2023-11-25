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
	"path"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/fluxcd/pkg/ssa"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/runtime"
)

var bundleApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Install or upgrade instances from a bundle",
	Long: `The bundle apply command installs or upgrades the instances defined in a bundle.
`,
	Example: `  # Install all instances from a bundle
  timoni bundle apply -f bundle.cue

  # Do a dry-run upgrade and print the diff
  timoni bundle apply -f bundle.cue \
  --dry-run --diff

  # Force apply instances from multiple bundles
  timoni bundle apply --force \
  -f ./bundle.cue \
  -f ./bundle_secrets.cue

  # Pass secret values from stdin
  cat ./bundle_secrets.cue | timoni bundle apply -f ./bundle.cue -f -
`,
	Args: cobra.NoArgs,
	RunE: runBundleApplyCmd,
}

type bundleApplyFlags struct {
	pkg                flags.Package
	files              []string
	dryrun             bool
	diff               bool
	wait               bool
	force              bool
	overwriteOwnership bool
	creds              flags.Credentials
}

var bundleApplyArgs bundleApplyFlags

func init() {
	bundleApplyCmd.Flags().VarP(&bundleApplyArgs.pkg, bundleApplyArgs.pkg.Type(), bundleApplyArgs.pkg.Shorthand(), bundleApplyArgs.pkg.Description())
	bundleApplyCmd.Flags().StringSliceVarP(&bundleApplyArgs.files, "file", "f", nil,
		"The local path to bundle.cue files.")
	bundleApplyCmd.Flags().BoolVar(&bundleApplyArgs.force, "force", false,
		"Recreate immutable Kubernetes resources.")
	bundleApplyCmd.Flags().BoolVar(&bundleApplyArgs.overwriteOwnership, "overwrite-ownership", false,
		"Overwrite instance ownership, if any instances are owned by other Bundles.")
	bundleApplyCmd.Flags().BoolVar(&bundleApplyArgs.dryrun, "dry-run", false,
		"Perform a server-side apply dry run.")
	bundleApplyCmd.Flags().BoolVar(&bundleApplyArgs.diff, "diff", false,
		"Perform a server-side apply dry run and prints the diff.")
	bundleApplyCmd.Flags().BoolVar(&bundleApplyArgs.wait, "wait", true,
		"Wait for the applied Kubernetes objects to become ready.")
	bundleApplyCmd.Flags().Var(&bundleApplyArgs.creds, bundleApplyArgs.creds.Type(), bundleApplyArgs.creds.Description())
	bundleCmd.AddCommand(bundleApplyCmd)
}

func runBundleApplyCmd(cmd *cobra.Command, _ []string) error {
	start := time.Now()
	files := bundleApplyArgs.files
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

	ctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	cuectx := cuecontext.New()
	bm := engine.NewBundleBuilder(cuectx, files)

	runtimeValues := make(map[string]string)

	if bundleArgs.runtimeFromEnv {
		maps.Copy(runtimeValues, engine.GetEnv())
	}

	rt, err := buildRuntime(bundleArgs.runtimeFiles)
	if err != nil {
		return err
	}

	clusters := rt.SelectClusters(bundleArgs.runtimeCluster, bundleArgs.runtimeClusterGroup)
	if len(clusters) == 0 {
		return errors.New("no cluster found")
	}

	ctxPull, cancel := context.WithTimeout(ctx, rootArgs.timeout)
	defer cancel()

	for _, cluster := range clusters {
		kubeconfigArgs.Context = &cluster.KubeContext

		clusterValues := make(map[string]string)

		// add values from env
		maps.Copy(clusterValues, runtimeValues)

		// add values from cluster
		rm, err := runtime.NewResourceManager(kubeconfigArgs)
		if err != nil {
			return err
		}
		reader := runtime.NewResourceReader(rm)
		rv, err := reader.Read(ctx, rt.Refs)
		if err != nil {
			return err
		}
		maps.Copy(clusterValues, rv)

		// add cluster info
		maps.Copy(clusterValues, cluster.NameGroupValues())

		// create cluster workspace
		workspace := path.Join(tmpDir, cluster.Name)
		if err := os.MkdirAll(workspace, os.ModePerm); err != nil {
			return err
		}

		if err := bm.InitWorkspace(workspace, clusterValues); err != nil {
			return describeErr(workspace, "failed to parse bundle", err)
		}

		v, err := bm.Build()
		if err != nil {
			return describeErr(tmpDir, "failed to build bundle", err)
		}

		bundle, err := bm.GetBundle(v)
		if err != nil {
			return err
		}

		log := LoggerBundle(cmd.Context(), bundle.Name, cluster.Name)

		if !bundleApplyArgs.overwriteOwnership {
			err = bundleInstancesOwnershipConflicts(bundle.Instances)
			if err != nil {
				return err
			}
		}

		for _, instance := range bundle.Instances {
			spin := StartSpinner(fmt.Sprintf("pulling %s", instance.Module.Repository))
			pullErr := fetchBundleInstanceModule(ctxPull, instance, tmpDir)
			spin.Stop()
			if pullErr != nil {
				return pullErr
			}
		}

		kubeVersion, err := runtime.ServerVersion(kubeconfigArgs)
		if err != nil {
			return err
		}

		startMsg := fmt.Sprintf("applying %v instance(s)", len(bundle.Instances))
		if !cluster.IsDefault() {
			startMsg = fmt.Sprintf("%s on %s", startMsg, colorizeSubject(cluster.Group))
		}

		if bundleApplyArgs.dryrun || bundleApplyArgs.diff {
			log.Info(fmt.Sprintf("%s %s", startMsg, colorizeDryRun("(server dry run)")))
		} else {
			log.Info(startMsg)
		}

		for _, instance := range bundle.Instances {
			instance.Cluster = cluster.Name
			if err := applyBundleInstance(logr.NewContext(ctx, log), cuectx, instance, kubeVersion, tmpDir); err != nil {
				return err
			}
		}

		elapsed := time.Since(start)
		if bundleApplyArgs.dryrun || bundleApplyArgs.diff {
			log.Info(fmt.Sprintf("applied successfully %s",
				colorizeDryRun("(server dry run)")))
		} else {
			log.Info(fmt.Sprintf("applied successfully in %s", elapsed.Round(time.Second)))
		}
	}
	return nil
}

func fetchBundleInstanceModule(ctx context.Context, instance *engine.BundleInstance, rootDir string) error {
	modDir := path.Join(rootDir, instance.Name)
	if err := os.MkdirAll(modDir, os.ModePerm); err != nil {
		return err
	}

	moduleVersion := instance.Module.Version
	if moduleVersion == apiv1.LatestVersion && instance.Module.Digest != "" {
		moduleVersion = "@" + instance.Module.Digest
	}

	fetcher := engine.NewFetcher(
		ctx,
		instance.Module.Repository,
		moduleVersion,
		modDir,
		rootArgs.cacheDir,
		bundleApplyArgs.creds.String(),
	)
	mod, err := fetcher.Fetch()
	if err != nil {
		return err
	}

	if instance.Module.Digest != "" && mod.Digest != instance.Module.Digest {
		return fmt.Errorf("the upstream digest %s of version %s doesn't match the specified digest %s",
			mod.Digest, instance.Module.Version, instance.Module.Digest)
	}

	instance.Module = *mod
	return nil
}

func applyBundleInstance(ctx context.Context, cuectx *cue.Context, instance *engine.BundleInstance, kubeVersion string, rootDir string) error {
	log := LoggerBundleInstance(ctx, instance.Bundle, instance.Cluster, instance.Name)

	modDir := path.Join(rootDir, instance.Name, "module")
	builder := engine.NewModuleBuilder(
		cuectx,
		instance.Name,
		instance.Namespace,
		modDir,
		bundleApplyArgs.pkg.String(),
	)

	if err := builder.WriteSchemaFile(); err != nil {
		return err
	}

	modName, err := builder.GetModuleName()
	if err != nil {
		return err
	}
	instance.Module.Name = modName

	log.Info(fmt.Sprintf("applying module %s version %s",
		colorizeSubject(instance.Module.Name), colorizeSubject(instance.Module.Version)))
	err = builder.WriteValuesFileWithDefaults(instance.Values)
	if err != nil {
		return err
	}

	builder.SetVersionInfo(instance.Module.Version, kubeVersion)

	buildResult, err := builder.Build()
	if err != nil {
		return describeErr(modDir, "build failed for "+instance.Name, err)
	}

	finalValues, err := builder.GetDefaultValues()
	if err != nil {
		return fmt.Errorf("failed to extract values: %w", err)
	}

	bundleApplySets, err := builder.GetApplySets(buildResult)
	if err != nil {
		return fmt.Errorf("failed to extract objects: %w", err)
	}

	var objects []*unstructured.Unstructured
	for _, set := range bundleApplySets {
		objects = append(objects, set.Objects...)
	}

	rm, err := runtime.NewResourceManager(kubeconfigArgs)
	if err != nil {
		return err
	}

	rm.SetOwnerLabels(objects, instance.Name, instance.Namespace)

	exists := false
	sm := runtime.NewStorageManager(rm)
	if _, err = sm.Get(ctx, instance.Name, instance.Namespace); err == nil {
		exists = true
	}

	nsExists, err := sm.NamespaceExists(ctx, instance.Namespace)
	if err != nil {
		return fmt.Errorf("instance init failed: %w", err)
	}

	im := runtime.NewInstanceManager(instance.Name, instance.Namespace, finalValues, instance.Module)

	if im.Instance.Labels == nil {
		im.Instance.Labels = make(map[string]string)
	}
	im.Instance.Labels[apiv1.BundleNameLabelKey] = instance.Bundle

	if err := im.AddObjects(objects); err != nil {
		return fmt.Errorf("adding objects to instance failed: %w", err)
	}

	staleObjects, err := sm.GetStaleObjects(ctx, &im.Instance)
	if err != nil {
		return fmt.Errorf("getting stale objects failed: %w", err)
	}

	if bundleApplyArgs.dryrun || bundleApplyArgs.diff {
		if !nsExists {
			log.Info(colorizeJoin(colorizeSubject("Namespace/"+instance.Namespace),
				ssa.CreatedAction, dryRunServer))
		}
		if err := instanceDryRunDiff(
			logr.NewContext(ctx, log),
			rm,
			objects,
			staleObjects,
			nsExists,
			rootDir,
			bundleApplyArgs.diff,
		); err != nil {
			return err
		}

		log.Info(colorizeJoin("applied successfully", colorizeDryRun("(server dry run)")))
		return nil
	}

	if !exists {
		log.Info(fmt.Sprintf("installing %s in namespace %s",
			colorizeSubject(instance.Name), colorizeSubject(instance.Namespace)))

		if err := sm.Apply(ctx, &im.Instance, true); err != nil {
			return fmt.Errorf("instance init failed: %w", err)
		}

		if !nsExists {
			log.Info(colorizeJoin(colorizeSubject("Namespace/"+instance.Namespace), ssa.CreatedAction))
		}
	} else {
		log.Info(fmt.Sprintf("upgrading %s in namespace %s",
			colorizeSubject(instance.Name), colorizeSubject(instance.Namespace)))
	}

	applyOpts := runtime.ApplyOptions(bundleApplyArgs.force, rootArgs.timeout)
	applyOpts.WaitInterval = 5 * time.Second

	waitOptions := ssa.WaitOptions{
		Interval: applyOpts.WaitInterval,
		Timeout:  rootArgs.timeout,
		FailFast: true,
	}

	for _, set := range bundleApplySets {
		if len(bundleApplySets) > 1 {
			log.Info(fmt.Sprintf("applying %s", set.Name))
		}

		cs, err := rm.ApplyAllStaged(ctx, set.Objects, applyOpts)
		if err != nil {
			return err
		}
		for _, change := range cs.Entries {
			log.Info(colorizeJoin(change))
		}

		if bundleApplyArgs.wait {
			spin := StartSpinner(fmt.Sprintf("waiting for %v resource(s) to become ready...", len(set.Objects)))
			err = rm.Wait(set.Objects, waitOptions)
			spin.Stop()
			if err != nil {
				return err
			}
			log.Info(fmt.Sprintf("%s resources %s", set.Name, colorizeReady("ready")))
		}
	}

	if images, err := builder.GetContainerImages(buildResult); err == nil {
		im.Instance.Images = images
	}

	if err := sm.Apply(ctx, &im.Instance, true); err != nil {
		return fmt.Errorf("storing instance failed: %w", err)
	}

	var deletedObjects []*unstructured.Unstructured
	if len(staleObjects) > 0 {
		deleteOpts := runtime.DeleteOptions(instance.Name, instance.Namespace)
		changeSet, err := rm.DeleteAll(ctx, staleObjects, deleteOpts)
		if err != nil {
			return fmt.Errorf("pruning objects failed: %w", err)
		}
		deletedObjects = runtime.SelectObjectsFromSet(changeSet, ssa.DeletedAction)
		for _, change := range changeSet.Entries {
			log.Info(colorizeJoin(change))
		}
	}

	if bundleApplyArgs.wait {
		if len(deletedObjects) > 0 {
			spin := StartSpinner(fmt.Sprintf("waiting for %v resource(s) to be finalized...", len(deletedObjects)))
			err = rm.WaitForTermination(deletedObjects, waitOptions)
			spin.Stop()
			if err != nil {
				return fmt.Errorf("waiting for termination failed: %w", err)
			}
		}
	}

	return nil
}

func bundleInstancesOwnershipConflicts(bundleInstances []*engine.BundleInstance) error {
	var conflicts []string
	rm, err := runtime.NewResourceManager(kubeconfigArgs)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	sm := runtime.NewStorageManager(rm)
	for _, instance := range bundleInstances {
		if existingInstance, err := sm.Get(ctx, instance.Name, instance.Namespace); err == nil {
			currentOwnerBundle := existingInstance.Labels[apiv1.BundleNameLabelKey]
			if currentOwnerBundle == "" {
				conflicts = append(conflicts, fmt.Sprintf("instance \"%s\" exists and is managed by no bundle", instance.Name))
			} else if currentOwnerBundle != instance.Bundle {
				conflicts = append(conflicts, fmt.Sprintf("instance \"%s\" exists and is managed by another bundle \"%s\"", instance.Name, currentOwnerBundle))
			}
		}
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("instance ownership conflicts encountered. Apply with \"--overwrite-ownership\" to gain instance ownership. Conflicts: %s", strings.Join(conflicts, "; "))
	}

	return nil
}

func saveReaderToFile(reader io.Reader) (string, error) {
	f, err := os.CreateTemp("", "*.cue")
	if err != nil {
		return "", errors.New("unable to create temp dir for stdin")
	}

	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return "", fmt.Errorf("error writing stdin to file: %w", err)
	}

	return f.Name(), nil
}
