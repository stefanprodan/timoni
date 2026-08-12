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

	"github.com/fluxcd/pkg/ssa"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/stefanprodan/timoni/internal/logger"
	"github.com/stefanprodan/timoni/internal/runtime"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

var deleteCmd = &cobra.Command{
	Use:     "delete INSTANCE_NAME",
	Args:    cobra.MaximumNArgs(1),
	Aliases: []string{"uninstall"},
	Short:   "Uninstall a module instance from the cluster",
	Long: `The delete command uninstalls the instance and deletes all its
Kubernetes resources from the cluster.

The release-state record (Secret/timoni.<instance>) is removed only after
every owned object is confirmed absent. If the termination cannot be
verified (--wait=false, a timeout, or an error), the record is retained as
a deletion-in-progress receipt and a subsequent delete finalizes the
removal from the exact retained inventory.`,
	Example: `  # Uninstall the app module from the default namespace
  timoni -n default delete app

  # Do a dry-run uninstall and print the changes
  timoni delete --dry-run app
`,
	RunE: runDeleteCmd,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		switch len(args) {
		case 0:
			return completeInstanceList(cmd, args, toComplete)
		default:
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	},
}

type deleteFlags struct {
	name   string
	dryrun bool
	wait   bool
}

var deleteArgs deleteFlags

func init() {
	deleteCmd.Flags().BoolVar(&deleteArgs.dryrun, "dry-run", false,
		"Perform a server-side delete dry run.")
	deleteCmd.Flags().BoolVar(&deleteArgs.wait, "wait", true,
		"Wait for the deleted Kubernetes objects to be finalized.")
	rootCmd.AddCommand(deleteCmd)
}

func runDeleteCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("name is required")
	}

	deleteArgs.name = args[0]

	log := loggerInstance(cmd.Context(), deleteArgs.name, true)
	sm, err := runtime.NewResourceManager(kubeconfigArgs)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	iStorage := runtime.NewStorageManager(sm)
	inst, err := iStorage.Get(ctx, deleteArgs.name, *kubeconfigArgs.Namespace)
	if err != nil {
		return err
	}

	iManager := runtime.InstanceManager{Instance: *inst}
	objects, err := iManager.ListObjects()
	if err != nil {
		return err
	}

	sort.Sort(sort.Reverse(ssa.SortableUnstructureds(objects)))

	if deleteArgs.dryrun {
		for _, object := range objects {
			log.Info(logger.ColorizeJoin(object, ssa.DeletedAction, logger.DryRunClient))
		}
		return nil
	}

	log.Info(fmt.Sprintf("deleting %v resource(s)...", len(objects)))
	return deleteInstanceObjects(ctx, log, sm, iStorage, inst, objects, deleteArgs.wait)
}

// deleteInstanceObjects sends delete requests for every object in the instance
// inventory and removes the release-state record only after the owned objects
// are confirmed absent. On timeout or unknown outcome the record is retained
// as a deletion-in-progress receipt so the deletion can be resumed.
func deleteInstanceObjects(ctx context.Context, log logr.Logger, sm *ssa.ResourceManager, iStorage *runtime.StorageManager, inst *apiv1.Instance, objects []*unstructured.Unstructured, wait bool) error {
	// Retain the release state as a deletion-in-progress receipt until every
	// owned object is confirmed absent. The instance data and exact inventory
	// are kept so a timed-out or interrupted delete can be resumed.
	if err := iStorage.SetDeleting(ctx, inst.Name, inst.Namespace); err != nil {
		return err
	}

	hasErrors := false
	cs := ssa.NewChangeSet()
	for _, object := range objects {
		deleteOpts := runtime.DeleteOptions(inst.Name, inst.Namespace)
		change, err := sm.Delete(ctx, object, deleteOpts)
		if err != nil {
			log.Error(err, "deletion failed")
			hasErrors = true
			continue
		}
		cs.Add(*change)
		log.Info(logger.ColorizeJoin(change))
	}

	if hasErrors {
		os.Exit(1)
	}

	deletedObjects := runtime.SelectObjectsFromSet(cs, ssa.DeletedAction)
	if wait && len(deletedObjects) > 0 {
		waitOpts := ssa.DefaultWaitOptions()
		waitOpts.Timeout = rootArgs.timeout
		spin := logger.StartSpinner(fmt.Sprintf("waiting for %v resource(s) to be finalized...", len(deletedObjects)))
		err := sm.WaitForTermination(deletedObjects, waitOpts)
		spin.Stop()
		if err != nil {
			// Keep the deletion-in-progress receipt so the delete can be resumed.
			return err
		}
		log.Info("all resources have been deleted")
	}

	// Drop the release state only after every exact object is proven absent
	// (termination wait succeeded) or when nothing was deleted at all. When
	// waiting is disabled, the owned objects may still be terminating, so the
	// receipt is retained for a subsequent delete to finish and remove it.
	if wait || len(deletedObjects) == 0 {
		return iStorage.Delete(ctx, inst.Name, inst.Namespace)
	}
	log.Info("delete requests issued, release state retained for recovery; re-run delete to finalize")
	return nil
}
