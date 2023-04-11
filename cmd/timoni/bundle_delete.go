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

	"cuelang.org/go/cue/cuecontext"
	"github.com/fluxcd/pkg/ssa"
	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/runtime"
)

var bundleDelCmd = &cobra.Command{
	Use:     "delete",
	Aliases: []string{"rm"},
	Short:   "Delete all instances from a bundle",
	Long: `The bundle delete command uninstalls the instances and
deletes all their Kubernetes resources from the cluster.'.
`,
	Example: `  # Uninstall all instances in a bundle
  timoni bundle delete -f bundle.cue

  # Uninstall all instances without waiting for finalisation
  timoni bundle delete -f bundle.cue --wait=false

  # Do a dry-run uninstall and print the changes
  timoni bundle delete -f bundle.cue --dry-run
`,
	RunE: runBundleDelCmd,
}

type bundleDelFlags struct {
	files  []string
	wait   bool
	dryrun bool
}

var bundleDelArgs bundleDelFlags

func init() {
	bundleDelCmd.Flags().StringSliceVarP(&bundleDelArgs.files, "file", "f", nil,
		"The local path to bundle.cue files.")
	bundleDelCmd.Flags().BoolVar(&bundleDelArgs.wait, "wait", true,
		"Wait for the deleted Kubernetes objects to be finalized.")
	bundleDelCmd.Flags().BoolVar(&bundleDelArgs.dryrun, "dry-run", false,
		"Perform a server-side delete dry run.")
	bundleCmd.AddCommand(bundleDelCmd)
}

func runBundleDelCmd(cmd *cobra.Command, args []string) error {
	bundleSchema, err := os.CreateTemp("", "schema.*.cue")
	if err != nil {
		return err
	}
	defer os.Remove(bundleSchema.Name())
	if _, err := bundleSchema.WriteString(apiv1.BundleSchema); err != nil {
		return err
	}

	files := append(bundleDelArgs.files, bundleSchema.Name())
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

	instances, err := bm.GetInstances(v)
	if err != nil {
		return err
	}

	if len(instances) == 0 {
		return fmt.Errorf("no instances found in bundle")
	}

	for _, instance := range instances {
		logger.Printf("deleting instance %s", instance.Name)
		if err := deleteBundleInstance(instance, bundleDelArgs.wait, bundleDelArgs.dryrun); err != nil {
			return err
		}
	}

	return nil
}

func deleteBundleInstance(instance engine.BundleInstance, wait bool, dryrun bool) error {
	sm, err := runtime.NewResourceManager(kubeconfigArgs)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	iStorage := runtime.NewStorageManager(sm)
	inst, err := iStorage.Get(ctx, instance.Name, instance.Namespace)
	if err != nil {
		return err
	}

	iManager := runtime.InstanceManager{Instance: *inst}
	objects, err := iManager.ListObjects()
	if err != nil {
		return err
	}

	sort.Sort(sort.Reverse(ssa.SortableUnstructureds(objects)))

	if dryrun {
		for _, object := range objects {
			logger.Println(fmt.Sprintf(
				"%s/%s/%s deleted (dry run)",
				object.GetKind(), object.GetNamespace(), object.GetName()))
		}
		return nil
	}

	logger.Println(fmt.Sprintf("deleting %v resource(s)...", len(objects)))
	hasErrors := false
	cs := ssa.NewChangeSet()
	for _, object := range objects {
		deleteOpts := runtime.DeleteOptions(instance.Name, instance.Namespace)
		change, err := sm.Delete(ctx, object, deleteOpts)
		if err != nil {
			logger.Println(`✗`, err)
			hasErrors = true
			continue
		}
		cs.Add(*change)
		logger.Println(change.String())
	}

	if hasErrors {
		os.Exit(1)
	}

	if err := iStorage.Delete(ctx, inst.Name, inst.Namespace); err != nil {
		return err
	}

	deletedObjects := runtime.SelectObjectsFromSet(cs, ssa.DeletedAction)
	if wait && len(deletedObjects) > 0 {
		waitOpts := ssa.DefaultWaitOptions()
		waitOpts.Timeout = rootArgs.timeout
		logger.Printf("waiting for %v resource(s) to be finalized...", len(deletedObjects))
		err = sm.WaitForTermination(deletedObjects, waitOpts)
		if err != nil {
			return err
		}
		logger.Println("all resources have been deleted")
	}

	return nil
}
