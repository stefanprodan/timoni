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
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
	bundleSchema, err := os.CreateTemp("", "schema.*.cue")
	if err != nil {
		return err
	}
	defer os.Remove(bundleSchema.Name())
	if _, err := bundleSchema.WriteString(apiv1.BundleSchema); err != nil {
		return err
	}

	files := append(bundleApplyArgs.files, bundleSchema.Name())
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

	ctx := cuecontext.New()
	bm := engine.NewBundleBuilder(ctx, files)

	v, err := bm.Build()
	if err != nil {
		return err
	}

	bundle, err := bm.GetBundle(v)
	if err != nil {
		return err
	}

	if !bundleApplyArgs.overwriteOwnership {
		err = bundleInstancesOwnershipConflicts(bundle.Instances)
		if err != nil {
			return err
		}
	}

	for _, instance := range bundle.Instances {
		logger.Printf("applying instance %s", instance.Name)
		if err := applyBundleInstance(instance); err != nil {
			return err
		}
	}

	return nil
}

func applyBundleInstance(instance engine.BundleInstance) error {
	moduleVersion := instance.Module.Version
	sourceURL := fmt.Sprintf("%s:%s", instance.Module.Repository, instance.Module.Version)

	if moduleVersion == engine.LatestTag && instance.Module.Digest != "" {
		sourceURL = fmt.Sprintf("%s@%s", instance.Module.Repository, instance.Module.Digest)
		moduleVersion = "@" + instance.Module.Digest
	}

	logger.Printf("pulling %s", sourceURL)

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctxPull, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	fetcher := engine.NewFetcher(
		ctxPull,
		instance.Module.Repository,
		moduleVersion,
		tmpDir,
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

	cuectx := cuecontext.New()
	builder := engine.NewModuleBuilder(
		cuectx,
		instance.Name,
		instance.Namespace,
		fetcher.GetModuleRoot(),
		bundleApplyArgs.pkg.String(),
	)

	if err := builder.WriteSchemaFile(); err != nil {
		return err
	}

	mod.Name, err = builder.GetModuleName()
	if err != nil {
		return err
	}

	logger.Printf("using module %s version %s", mod.Name, mod.Version)

	err = builder.WriteValuesFile(instance.Values)
	if err != nil {
		return err
	}

	buildResult, err := builder.Build()
	if err != nil {
		return describeErr(fetcher.GetModuleRoot(), "failed to build instance", err)
	}

	finalValues, err := builder.GetValues(buildResult)
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

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	exists := false
	sm := runtime.NewStorageManager(rm)
	if _, err = sm.Get(ctx, instance.Name, instance.Namespace); err == nil {
		exists = true
	}

	if bundleApplyArgs.dryrun || bundleApplyArgs.diff {
		diffOpts := ssa.DefaultDiffOptions()
		sort.Sort(ssa.SortableUnstructureds(objects))
		for _, r := range objects {
			change, liveObject, mergedObject, err := rm.Diff(ctx, r, diffOpts)
			if err != nil {
				logger.Println(err)
				continue
			}

			logger.Println(change.String(), "(server dry run)")

			if bundleApplyArgs.diff && change.Action == ssa.ConfiguredAction {
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

				out, _ := exec.Command("diff", "-N", "-u", liveFile, mergedFile).Output()
				for i, line := range strings.Split(string(out), "\n") {
					if i > 1 && len(line) > 0 {
						logger.Println(line)
					}
				}
			}
		}

		logger.Println("bundle applied successfully")
		return nil
	}

	im := runtime.NewInstanceManager(instance.Name, instance.Namespace, finalValues, *mod)

	if im.Instance.Labels == nil {
		im.Instance.Labels = make(map[string]string)
	}
	im.Instance.Labels[apiv1.BundleNameLabelKey] = instance.Bundle

	if err := im.AddObjects(objects); err != nil {
		return fmt.Errorf("adding objects to instance failed: %w", err)
	}

	if !exists {
		logger.Printf("installing %s in namespace %s", instance.Name, instance.Namespace)

		nsExists, err := sm.NamespaceExists(ctx, instance.Namespace)
		if err != nil {
			return fmt.Errorf("instance init failed: %w", err)
		}

		if err := sm.Apply(ctx, &im.Instance, true); err != nil {
			return fmt.Errorf("instance init failed: %w", err)
		}

		if !nsExists {
			logger.Printf("Namespace/%s created", instance.Namespace)
		}
	} else {
		logger.Printf("upgrading %s in namespace %s", instance.Name, instance.Namespace)
	}

	bundleApplyOpts := runtime.ApplyOptions(bundleApplyArgs.force, time.Minute)

	for _, set := range bundleApplySets {
		if len(bundleApplySets) > 1 {
			logger.Println("applying", set.Name)
		}

		cs, err := rm.ApplyAllStaged(ctx, set.Objects, bundleApplyOpts)
		if err != nil {
			return err
		}
		for _, change := range cs.Entries {
			logger.Println(change.String())
		}

		if bundleApplyArgs.wait {
			logger.Println(fmt.Sprintf("waiting for %v resource(s) to become ready...", len(set.Objects)))
			err = rm.Wait(set.Objects, ssa.DefaultWaitOptions())
			if err != nil {
				return err
			}
			logger.Println("resources are ready")
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
		deleteOpts := runtime.DeleteOptions(instance.Name, instance.Namespace)
		changeSet, err := rm.DeleteAll(ctx, staleObjects, deleteOpts)
		if err != nil {
			return fmt.Errorf("prunning objects failed: %w", err)
		}
		deletedObjects = runtime.SelectObjectsFromSet(changeSet, ssa.DeletedAction)
		for _, change := range changeSet.Entries {
			logger.Println(change.String())
		}
	}

	if bundleApplyArgs.wait {
		if len(deletedObjects) > 0 {
			logger.Printf("waiting for %v resource(s) to be finalized...", len(deletedObjects))
			err = rm.WaitForTermination(deletedObjects, ssa.DefaultWaitOptions())
			if err != nil {
				return fmt.Errorf("wating for termination failed: %w", err)
			}

			logger.Println("all resources are ready")
		}
	}

	return nil
}

func bundleInstancesOwnershipConflicts(bundleInstances []engine.BundleInstance) error {
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
		return "", fmt.Errorf("unable to create temp dir for stdin")
	}

	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return "", fmt.Errorf("error writing stdin to file: %w", err)
	}

	return f.Name(), nil
}
