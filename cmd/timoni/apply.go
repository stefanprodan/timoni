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
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cuelang.org/go/cue/cuecontext"
	"github.com/fluxcd/pkg/oci"
	"github.com/fluxcd/pkg/ssa"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/runtime"
)

var applyCmd = &cobra.Command{
	Use:     "apply [INSTANCE NAME] [MODULE URL]",
	Aliases: []string{"install", "upgrade"},
	Short:   "Install or upgrade a module instance",
	Long: `The apply command installs or upgrades a module instance on the Kubernetes cluster.

The apply command performs the following steps:

- Pulls the module version from the specified container registry.
- If the registry is private, uses the credentials found in '~/.docker/config.json'.
- If the registry credentials are specified with '--creds', these take priority over the docker ones.
- Creates the specified '--namespace' if it doesn't exist.
- Merges all the values supplied with '--values' on top of the default values found in the module.
- Builds the module by passing the instance name, namespace and values.
- Labels the resulting Kubernetes resources with the instance name and namespace.
- Applies the Kubernetes resources on the cluster.
- Creates or updates the instance inventory with the last applied resources IDs.
- Recreates the resources annotated with 'action.timoni.sh/force: "enabled"' if they contain changes to immutable fields.
- Waits for the applied resources to become ready.
- Deletes the resources which were previously applied but are missing from the current instance.
- Skips the resources annotated with 'action.timoni.sh/prune: "disabled"' from deletion.
- Waits for the deleted resources to be finalised.
`,
	Example: `  # Install a module instance and create the namespace if it doesn't exists
  timoni apply -n apps app oci://docker.io/org/module -v 1.0.0

  # Do a dry-run upgrade and print the diff
  timoni apply -n apps app oci://docker.io/org/module -v 1.0.0 \
  --values ./values-1.cue \
  --dry-run --diff

  # Install or upgrade an instance with custom values by merging them in the specified order
  timoni apply -n apps app oci://docker.io/org/module -v 1.0.0 \
  --values ./values-1.cue \
  --values ./values-2.cue

  # Upgrade an instance and recreate immutable Kubernetes resources such as Jobs
  timoni apply -n apps app oci://docker.io/org/module -v 2.0.0 \
  --values ./values-1.cue \
  --force

  # Install or upgrade an instance with custom values from stdin
  echo "values: replicas: 2" | timoni apply -n apps app oci://docker.io/org/module --values -

  # Install or upgrade an instance with values in YAML and JSON format
  timoni apply -n apps app oci://docker.io/org/module \
  --values ./values-1.yaml \
  --values ./values-2.json
`,
	RunE: runApplyCmd,
}

type applyFlags struct {
	name               string
	module             string
	version            flags.Version
	pkg                flags.Package
	valuesFiles        []string
	dryrun             bool
	diff               bool
	wait               bool
	force              bool
	overwriteOwnership bool
	creds              flags.Credentials
}

var applyArgs applyFlags

func init() {
	applyCmd.Flags().VarP(&applyArgs.version, applyArgs.version.Type(), applyArgs.version.Shorthand(), applyArgs.version.Description())
	applyCmd.Flags().VarP(&applyArgs.pkg, applyArgs.pkg.Type(), applyArgs.pkg.Shorthand(), applyArgs.pkg.Description())
	applyCmd.Flags().StringSliceVarP(&applyArgs.valuesFiles, "values", "f", nil,
		"The local path to values files (cue, yaml or json format).")
	applyCmd.Flags().BoolVar(&applyArgs.force, "force", false,
		"Recreate immutable Kubernetes resources.")
	applyCmd.Flags().BoolVar(&applyArgs.overwriteOwnership, "overwrite-ownership", false,
		"Overwrite instance ownership, if the instance is owned by a Bundle.")
	applyCmd.Flags().BoolVar(&applyArgs.dryrun, "dry-run", false,
		"Perform a server-side apply dry run.")
	applyCmd.Flags().BoolVar(&applyArgs.diff, "diff", false,
		"Perform a server-side apply dry run and prints the diff.")
	applyCmd.Flags().BoolVar(&applyArgs.wait, "wait", true,
		"Wait for the applied Kubernetes objects to become ready.")
	applyCmd.Flags().Var(&applyArgs.creds, applyArgs.creds.Type(), applyArgs.creds.Description())
	rootCmd.AddCommand(applyCmd)
}

func runApplyCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("name and module are required")
	}

	applyArgs.name = args[0]
	applyArgs.module = args[1]

	log := LoggerFrom(cmd.Context(), "instance", applyArgs.name)

	version := applyArgs.version.String()
	if version == "" {
		version = engine.LatestTag
	}

	if strings.HasPrefix(applyArgs.module, oci.OCIRepositoryPrefix) {
		log.Info(fmt.Sprintf("pulling %s:%s", applyArgs.module, version))
	} else {
		log.Info(fmt.Sprintf("building %s", applyArgs.module))
	}

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctxPull, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	fetcher := engine.NewFetcher(
		ctxPull,
		applyArgs.module,
		version,
		tmpDir,
		applyArgs.creds.String(),
	)
	mod, err := fetcher.Fetch()
	if err != nil {
		return err
	}

	cuectx := cuecontext.New()
	builder := engine.NewModuleBuilder(
		cuectx,
		applyArgs.name,
		*kubeconfigArgs.Namespace,
		fetcher.GetModuleRoot(),
		applyArgs.pkg.String(),
	)

	if err := builder.WriteSchemaFile(); err != nil {
		return err
	}

	mod.Name, err = builder.GetModuleName()
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("using module %s version %s", mod.Name, mod.Version))

	if len(applyArgs.valuesFiles) > 0 {
		valuesCue, err := convertToCue(cmd, applyArgs.valuesFiles)
		if err != nil {
			return err
		}
		err = builder.MergeValuesFile(valuesCue)
		if err != nil {
			return err
		}
	}

	buildResult, err := builder.Build()
	if err != nil {
		return describeErr(fetcher.GetModuleRoot(), "failed to build instance", err)
	}

	finalValues, err := builder.GetValues(buildResult)
	if err != nil {
		return fmt.Errorf("failed to extract values: %w", err)
	}

	applySets, err := builder.GetApplySets(buildResult)
	if err != nil {
		return fmt.Errorf("failed to extract objects: %w", err)
	}

	var objects []*unstructured.Unstructured
	for _, set := range applySets {
		objects = append(objects, set.Objects...)
	}

	rm, err := runtime.NewResourceManager(kubeconfigArgs)
	if err != nil {
		return err
	}

	rm.SetOwnerLabels(objects, applyArgs.name, *kubeconfigArgs.Namespace)

	ctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	exists := false
	sm := runtime.NewStorageManager(rm)
	instance, err := sm.Get(ctx, applyArgs.name, *kubeconfigArgs.Namespace)
	if err == nil {
		exists = true
	}

	nsExists, err := sm.NamespaceExists(ctx, *kubeconfigArgs.Namespace)
	if err != nil {
		return fmt.Errorf("instance init failed: %w", err)
	}

	if !applyArgs.overwriteOwnership && exists {
		err = instanceOwnershipConflicts(*instance)
		if err != nil {
			return err
		}
	}

	if applyArgs.dryrun || applyArgs.diff {
		if !nsExists {
			log.Info(fmt.Sprintf("Namespace/%s created (server dry run)", *kubeconfigArgs.Namespace))
		}
		return instanceDryRun(logr.NewContext(ctx, log), rm, objects, nsExists, tmpDir, applyArgs.diff)
	}

	im := runtime.NewInstanceManager(applyArgs.name, *kubeconfigArgs.Namespace, finalValues, *mod)

	if err := im.AddObjects(objects); err != nil {
		return fmt.Errorf("adding objects to instance failed: %w", err)
	}

	if !exists {
		log.Info(fmt.Sprintf("installing %s in namespace %s", applyArgs.name, *kubeconfigArgs.Namespace))

		if err := sm.Apply(ctx, &im.Instance, true); err != nil {
			return fmt.Errorf("instance init failed: %w", err)
		}

		if !nsExists {
			log.Info(fmt.Sprintf("Namespace/%s created", *kubeconfigArgs.Namespace))
		}
	} else {
		log.Info(fmt.Sprintf("upgrading %s in namespace %s", applyArgs.name, *kubeconfigArgs.Namespace))
	}

	applyOpts := runtime.ApplyOptions(applyArgs.force, rootArgs.timeout)
	waitOptions := ssa.WaitOptions{
		Interval: 5 * time.Second,
		Timeout:  rootArgs.timeout,
	}

	for _, set := range applySets {
		if len(applySets) > 1 {
			log.Info(fmt.Sprintf("applying %s", set.Name))
		}

		cs, err := rm.ApplyAllStaged(ctx, set.Objects, applyOpts)
		if err != nil {
			return err
		}
		for _, change := range cs.Entries {
			log.Info(change.String())
		}

		if applyArgs.wait {
			spin := StartSpinner(fmt.Sprintf("waiting for %v resource(s) to become ready...", len(set.Objects)))
			err = rm.Wait(set.Objects, waitOptions)
			spin.Stop()
			if err != nil {
				return err
			}
			log.Info("resources are ready")
		}
	}

	staleObjects, err := sm.GetStaleObjects(ctx, &im.Instance)
	if err != nil {
		return fmt.Errorf("getting stale objects failed: %w", err)
	}

	if err := sm.Apply(ctx, &im.Instance, true); err != nil {
		return fmt.Errorf("storing instance failed: %w", err)
	}

	var deletedObjects []*unstructured.Unstructured
	if len(staleObjects) > 0 {
		deleteOpts := runtime.DeleteOptions(applyArgs.name, *kubeconfigArgs.Namespace)
		changeSet, err := rm.DeleteAll(ctx, staleObjects, deleteOpts)
		if err != nil {
			return fmt.Errorf("prunning objects failed: %w", err)
		}
		deletedObjects = runtime.SelectObjectsFromSet(changeSet, ssa.DeletedAction)
		for _, change := range changeSet.Entries {
			log.Info(change.String())
		}
	}

	if applyArgs.wait {
		if len(deletedObjects) > 0 {
			spin := StartSpinner(fmt.Sprintf("waiting for %v resource(s) to be finalized...", len(deletedObjects)))
			err = rm.WaitForTermination(deletedObjects, waitOptions)
			spin.Stop()
			if err != nil {
				return fmt.Errorf("wating for termination failed: %w", err)
			}

			log.Info("all resources are ready")
		}
	}

	return nil
}

func instanceOwnershipConflicts(instance apiv1.Instance) error {
	if currentOwnerBundle := instance.Labels[apiv1.BundleNameLabelKey]; currentOwnerBundle != "" {
		return fmt.Errorf("instance ownership conflict encountered. Apply with \"--overwrite-ownership\" to gain instance ownership. Conflict: instance \"%s\" exists and is managed by bundle \"%s\"", instance.Name, currentOwnerBundle)
	}
	return nil
}

func instanceDryRun(ctx context.Context,
	rm *ssa.ResourceManager,
	objects []*unstructured.Unstructured,
	nsExists bool,
	tmpDir string,
	withDiff bool) error {
	log := LoggerFrom(ctx)
	diffOpts := ssa.DefaultDiffOptions()
	sort.Sort(ssa.SortableUnstructureds(objects))

	for _, r := range objects {
		if !nsExists {
			log.Info(fmt.Sprintf("%s created (server dry run)", ssa.FmtUnstructured(r)))
			continue
		}

		change, liveObject, mergedObject, err := rm.Diff(ctx, r, diffOpts)
		if err != nil {
			log.Error(err, "diff failed")
			continue
		}

		log.Info(fmt.Sprintf("%s (server dry run)", change.String()))
		if withDiff && change.Action == ssa.ConfiguredAction {
			liveYAML, _ := yaml.Marshal(liveObject)
			liveFile := filepath.Join(tmpDir, "live.yaml")
			if err := os.WriteFile(liveFile, liveYAML, 0644); err != nil {
				return err
			}

			mergedYAML, _ := yaml.Marshal(mergedObject)
			mergedFile := filepath.Join(tmpDir, "merged.yaml")
			if err := os.WriteFile(mergedFile, mergedYAML, 0644); err != nil {
				return err
			}

			if err := diffYAML(liveFile, mergedFile, rootCmd.OutOrStdout()); err != nil {
				return err
			}
		}
	}
	return nil
}
