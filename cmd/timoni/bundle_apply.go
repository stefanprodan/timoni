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
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/load"

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
`,
	RunE: runBundleApplyCmd,
}

type bundleApplyFlags struct {
	pkg    flags.Package
	files  []string
	dryrun bool
	diff   bool
	wait   bool
	force  bool
	creds  flags.Credentials
}

var bundleApplyArgs bundleApplyFlags

func init() {
	bundleApplyCmd.Flags().VarP(&bundleApplyArgs.pkg, bundleApplyArgs.pkg.Type(), bundleApplyArgs.pkg.Shorthand(), bundleApplyArgs.pkg.Description())
	bundleApplyCmd.Flags().StringSliceVarP(&bundleApplyArgs.files, "file", "f", nil,
		"The local path to bundle.cue files.")
	bundleApplyCmd.Flags().BoolVar(&bundleApplyArgs.force, "force", false,
		"Recreate immutable Kubernetes resources.")
	bundleApplyCmd.Flags().BoolVar(&bundleApplyArgs.dryrun, "dry-run", false,
		"Perform a server-side apply dry run.")
	bundleApplyCmd.Flags().BoolVar(&bundleApplyArgs.diff, "diff", false,
		"Perform a server-side apply dry run and prints the diff.")
	bundleApplyCmd.Flags().BoolVar(&bundleApplyArgs.wait, "wait", true,
		"Wait for the applied Kubernetes objects to become ready.")
	bundleApplyCmd.Flags().Var(&bundleApplyArgs.creds, bundleApplyArgs.creds.Type(), bundleApplyArgs.creds.Description())
	bundleCmd.AddCommand(bundleApplyCmd)
}

func runBundleApplyCmd(cmd *cobra.Command, args []string) error {
	bundleSchema, err := os.CreateTemp("", "schema.*.cue")
	if err != nil {
		return err
	}
	defer os.Remove(bundleSchema.Name())
	if _, err := bundleSchema.WriteString(apiv1.BundleSchema); err != nil {
		return err
	}

	ctx := cuecontext.New()

	cfg := &load.Config{
		Package:   "_",
		DataFiles: true,
	}

	files := append(bundleApplyArgs.files, bundleSchema.Name())
	ix := load.Instances(files, cfg)
	if len(ix) == 0 {
		return fmt.Errorf("no bundle found")
	}

	inst := ix[0]
	if inst.Err != nil {
		return fmt.Errorf("bundle error: %w", inst.Err)
	}

	v := ctx.BuildInstance(inst)
	if v.Err() != nil {
		return v.Err()
	}

	apiVersion := v.LookupPath(cue.ParsePath(apiv1.BundleAPIVersionSelector.String()))
	if apiVersion.Err() != nil {
		return fmt.Errorf("lookup %s failed, error: %w", apiv1.BundleAPIVersionSelector.String(), apiVersion.Err())
	}

	apiVer, _ := apiVersion.String()
	if apiVer != apiv1.GroupVersion.Version {
		return fmt.Errorf("API version %s not supported, must be %s", apiVer, apiv1.GroupVersion.Version)
	}

	instances := v.LookupPath(cue.ParsePath(apiv1.BundleInstancesSelector.String()))
	if instances.Err() != nil {
		return fmt.Errorf("lookup %s failed, error: %w", apiv1.BundleInstancesSelector.String(), instances.Err())
	}

	iter, _ := instances.Fields(cue.Concrete(true))
	for iter.Next() {
		name := iter.Selector().String()
		expr := iter.Value()

		logger.Printf("applying instance %s", name)

		moduleURL := expr.LookupPath(cue.ParsePath(apiv1.BundleModuleURLSelector.String()))
		if moduleURL.Err() != nil {
			return fmt.Errorf("lookup %s failed, error: %w", apiv1.BundleModuleURLSelector.String(), instances.Err())
		}

		moduleVersion := expr.LookupPath(cue.ParsePath(apiv1.BundleModuleVersionSelector.String()))
		if moduleVersion.Err() != nil {
			return fmt.Errorf("lookup %s failed, error: %w", apiv1.BundleModuleVersionSelector.String(), instances.Err())
		}

		namespace := expr.LookupPath(cue.ParsePath(apiv1.BundleNamespaceSelector.String()))
		if namespace.Err() != nil {
			return fmt.Errorf("lookup %s failed, error: %w", apiv1.BundleNamespaceSelector.String(), instances.Err())
		}

		values := expr.LookupPath(cue.ParsePath(apiv1.BundleValuesSelector.String()))
		if values.Err() != nil {
			return fmt.Errorf("lookup %s failed, error: %w", apiv1.BundleValuesSelector.String(), instances.Err())
		}

		ns, _ := namespace.String()
		url, _ := moduleURL.String()
		version, _ := moduleVersion.String()

		err := applyBundleInstance(name, ns, url, version, values)
		if err != nil {
			return err
		}
	}

	return nil
}

func applyBundleInstance(name, namespace, moduleURL, moduleVersion string, values cue.Value) error {
	logger.Printf("pulling %s:%s", moduleURL, moduleVersion)

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctxPull, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	fetcher := engine.NewFetcher(
		ctxPull,
		moduleURL,
		moduleVersion,
		tmpDir,
		bundleApplyArgs.creds.String(),
	)
	mod, err := fetcher.Fetch()
	if err != nil {
		return err
	}

	cuectx := cuecontext.New()
	builder := engine.NewModuleBuilder(
		cuectx,
		name,
		namespace,
		fetcher.GetModuleRoot(),
		bundleApplyArgs.pkg.String(),
	)

	mod.Name, err = builder.GetModuleName()
	if err != nil {
		return err
	}

	logger.Printf("using module %s version %s", mod.Name, mod.Version)

	err = builder.WriteValuesFile(values)
	if err != nil {
		return err
	}

	buildResult, err := builder.Build()
	if err != nil {
		return fmt.Errorf("failed to build instance, error: %w", err)
	}

	apiVer, err := builder.GetAPIVersion(buildResult)
	if err != nil {
		return err
	}

	if apiVer != apiv1.GroupVersion.Version {
		return fmt.Errorf("API version %s not supported, must be %s", apiVer, apiv1.GroupVersion.Version)
	}

	finalValues, err := builder.GetValues(buildResult)
	if err != nil {
		return fmt.Errorf("failed to extract values, error: %w", err)
	}

	bundleApplySets, err := builder.GetApplySets(buildResult)
	if err != nil {
		return fmt.Errorf("failed to extract objects, error: %w", err)
	}

	var objects []*unstructured.Unstructured
	for _, set := range bundleApplySets {
		objects = append(objects, set.Objects...)
	}

	rm, err := runtime.NewResourceManager(kubeconfigArgs)
	if err != nil {
		return err
	}

	rm.SetOwnerLabels(objects, name, namespace)

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	exists := false
	sm := runtime.NewStorageManager(rm)
	if _, err := sm.Get(ctx, name, namespace); err == nil {
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

		logger.Println("bundled applied successfully")
		return nil
	}

	im := runtime.NewInstanceManager(name, namespace, finalValues, *mod)

	if err := im.AddObjects(objects); err != nil {
		return fmt.Errorf("adding objects to instance failed, error: %w", err)
	}

	if !exists {
		logger.Printf("installing %s in namespace %s", name, namespace)

		nsExists, err := sm.NamespaceExists(ctx, namespace)
		if err != nil {
			return fmt.Errorf("instance init failed, error: %w", err)
		}

		if err := sm.Apply(ctx, &im.Instance, true); err != nil {
			return fmt.Errorf("instance init failed, error: %w", err)
		}

		if !nsExists {
			logger.Printf("Namespace/%s created", namespace)
		}
	} else {
		logger.Printf("upgrading %s in namespace %s", name, namespace)
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
		return fmt.Errorf("getting stale objects failed, error: %w", err)
	}

	if err := sm.Apply(ctx, &im.Instance, true); err != nil {
		return fmt.Errorf("storing instance failed, error: %w", err)
	}

	var deletedObjects []*unstructured.Unstructured
	if len(staleObjects) > 0 {
		deleteOpts := runtime.DeleteOptions(name, namespace)
		changeSet, err := rm.DeleteAll(ctx, staleObjects, deleteOpts)
		if err != nil {
			return fmt.Errorf("prunning objects failed, error: %w", err)
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
				return fmt.Errorf("wating for termination failed, error: %w", err)
			}

			logger.Println("all resources are ready")
		}
	}

	return nil
}
