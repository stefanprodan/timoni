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
	"github.com/spf13/cobra"

	"github.com/stefanprodan/timoni/internal/runtime"
)

var deleteCmd = &cobra.Command{
	Use:     "delete [INSTANCE NAME]",
	Aliases: []string{"uninstall"},
	Short:   "Uninstall a module from the cluster",
	Example: `  # Uninstall the app module from the default namespace
  timoni -n default delete app

  # Do a dry-run uninstall and print the changes
  timoni delete --dry-run app
`,
	RunE: runDeleteCmd,
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

	sm, err := runtime.NewResourceManager(kubeconfigArgs)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
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
			logger.Println(fmt.Sprintf(
				"%s/%s/%s deleted (dry run)",
				object.GetKind(), object.GetNamespace(), object.GetName()))
		}
		return nil
	}

	logger.Println(fmt.Sprintf("deleting %v resource(s)...", len(objects)))
	hasErrors := false
	for _, object := range objects {
		change, err := sm.Delete(ctx, object, ssa.DefaultDeleteOptions())
		if err != nil {
			logger.Println(`✗`, err)
			hasErrors = true
			continue
		}
		logger.Println(change.String())
	}

	if hasErrors {
		os.Exit(1)
	}

	if err := iStorage.Delete(ctx, inst.Name, inst.Namespace); err != nil {
		return err
	}

	if deleteArgs.wait {
		waitOpts := ssa.DefaultWaitOptions()
		waitOpts.Timeout = rootArgs.timeout
		logger.Println("waiting for resources to be terminated...")
		err = sm.WaitForTermination(objects, waitOpts)
		if err != nil {
			return err
		}
		logger.Println("all resources have been deleted")
	}

	return nil
}
