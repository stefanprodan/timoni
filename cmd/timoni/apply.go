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
	"cuelang.org/go/cue/cuecontext"
	"fmt"
	"github.com/fluxcd/pkg/ssa"
	"github.com/spf13/cobra"
	"github.com/stefanprodan/timoni/pkg/inventory"
	"os"
	"os/exec"
	"path/filepath"
	"sigs.k8s.io/yaml"
	"sort"
	"strings"
)

var applyCmd = &cobra.Command{
	Use:     "apply [NAME] [MODULE]",
	Aliases: []string{"install", "upgrade"},
	Short:   "Install or upgrade a module.",
	Example: `  # install a local module with its default values and create the namespace if it doesn't exists
  timoni apply app ./path/to/module --namespace apps

  # do a dry-run apply and print the diff
  timoni apply -n apps --dry-run --diff app ./path/to/module \
  --values ./values-1.cue 

  # apply a module with custom values by merging them in the specified order
  timoni apply -n apps -n apps app ./path/to/module \
  --values ./values-1.cue \
  --values ./values-2.cue
`,
	RunE: runApplyCmd,
}

type applyFlags struct {
	name        string
	module      string
	pkg         string
	valuesFiles []string
	dryrun      bool
	diff        bool
	wait        bool
}

var applyArgs applyFlags

func init() {
	applyCmd.Flags().StringVarP(&applyArgs.pkg, "package", "p", "main",
		"The name of the package containing the instance values and resources.")
	applyCmd.Flags().StringSliceVarP(&applyArgs.valuesFiles, "values", "f", nil, "local path to values.cue files")
	applyCmd.Flags().BoolVar(&applyArgs.dryrun, "dry-run", false,
		"performs a server-side apply dry run")
	applyCmd.Flags().BoolVar(&applyArgs.diff, "diff", false,
		"performs a server-side apply dry run and prints the diff")
	applyCmd.Flags().BoolVar(&applyArgs.wait, "wait", true,
		"wait for the applied Kubernetes objects to become ready")
	rootCmd.AddCommand(applyCmd)
}

func runApplyCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("name and module are required")
	}

	applyArgs.name = args[0]
	applyArgs.module = args[1]

	if _, err := os.Stat(applyArgs.module); err != nil {
		return fmt.Errorf("package not found at path %s", applyArgs.module)
	}

	logger.Println("building", applyArgs.module)

	tmpDir, err := os.MkdirTemp("", "timoni")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	modulePath := filepath.Join(tmpDir, "module")
	err = copyModule(applyArgs.module, modulePath)
	if err != nil {
		return err
	}

	cuectx := cuecontext.New()
	builder := NewBuilder(cuectx, applyArgs.name, *kubeconfigArgs.Namespace, modulePath, applyArgs.pkg)

	if len(applyArgs.valuesFiles) > 0 {
		err = builder.MergeValuesFile(applyArgs.valuesFiles)
		if err != nil {
			return err
		}
	}

	buildResult, err := builder.Build()
	if err != nil {
		return fmt.Errorf("failed to build instance, error: %w", err)
	}

	objects, err := builder.GetObjects(buildResult)
	if err != nil {
		return fmt.Errorf("failed to extract resouces, error: %w", err)
	}

	so := ssa.Owner{
		Field: "timoni",
		Group: "timoni.mod",
	}

	sm, err := newManager(so)
	if err != nil {
		return err
	}

	sm.SetOwnerLabels(objects, applyArgs.name, *kubeconfigArgs.Namespace)

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	if applyArgs.dryrun {
		diffOpts := ssa.DefaultDiffOptions()
		sort.Sort(ssa.SortableUnstructureds(objects))
		for _, r := range objects {
			change, liveObject, mergedObject, err := sm.Diff(ctx, r, diffOpts)
			if err != nil {
				logger.Println(err)
				continue
			}

			logger.Println(change.String(), "(server dry run)")

			if applyArgs.diff && change.Action == string(ssa.ConfiguredAction) {
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
		return nil
	}

	invStorage := &inventory.Storage{Manager: sm, Owner: so}
	newInventory := inventory.NewInventory(applyArgs.name, *kubeconfigArgs.Namespace)
	newInventory.SetSource(applyArgs.module, applyArgs.module, []string{})
	if err := newInventory.AddObjects(objects); err != nil {
		return fmt.Errorf("creating inventory failed, error: %w", err)
	}

	cs, err := sm.ApplyAllStaged(ctx, objects, ssa.DefaultApplyOptions())
	if err != nil {
		return err
	}
	for _, change := range cs.Entries {
		logger.Println(change.String())
	}

	staleObjects, err := invStorage.GetInventoryStaleObjects(ctx, newInventory)
	if err != nil {
		return fmt.Errorf("inventory query failed, error: %w", err)
	}

	err = invStorage.ApplyInventory(ctx, newInventory, true)
	if err != nil {
		return fmt.Errorf("inventory apply failed, error: %w", err)
	}

	if len(staleObjects) > 0 {
		changeSet, err := sm.DeleteAll(ctx, staleObjects, ssa.DefaultDeleteOptions())
		if err != nil {
			return fmt.Errorf("prune failed, error: %w", err)
		}
		for _, change := range changeSet.Entries {
			logger.Println(change.String())
		}
	}

	if applyArgs.wait {
		logger.Println(fmt.Sprintf("waiting for %v resource(s) to become ready...", len(objects)))
		err = sm.Wait(objects, ssa.DefaultWaitOptions())
		if err != nil {
			return err
		}

		if len(staleObjects) > 0 {
			logger.Println(fmt.Sprintf("waiting for %v resource(s) to be finalized...", len(staleObjects)))
			err = sm.WaitForTermination(staleObjects, ssa.DefaultWaitOptions())
			if err != nil {
				return fmt.Errorf("wating for termination failed, error: %w", err)
			}
		}

		logger.Println("all resources are ready")
	}

	return nil
}
