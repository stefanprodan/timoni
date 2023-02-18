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

	"github.com/fluxcd/pkg/ssa"
	"github.com/spf13/cobra"
	"github.com/stefanprodan/timoni/internal/runtime"
)

var inspectResourcesCmd = &cobra.Command{
	Use:   "resources [INSTANCE NAME]",
	Short: "Print the Kubernetes objects managed by an instance",
	Example: `  # Print the managed resources
  timoni -n default inspect resources app
`,
	RunE: runInspectResourcesCmd,
}

type inspectResourcesFlags struct {
	name string
}

var inspectResourcesArgs inspectResourcesFlags

func init() {
	inspectCmd.AddCommand(inspectResourcesCmd)
}

func runInspectResourcesCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("instance name is required")
	}
	inspectResourcesArgs.name = args[0]

	sm, err := runtime.NewResourceManager(kubeconfigArgs)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	iStorage := runtime.NewStorageManager(sm)
	inst, err := iStorage.Get(ctx, inspectResourcesArgs.name, *kubeconfigArgs.Namespace)
	if err != nil {
		return err
	}

	iManager := runtime.InstanceManager{Instance: *inst}

	metas, err := iManager.ListMeta()
	if err != nil {
		return err
	}

	for _, meta := range metas {
		cmd.OutOrStdout().Write([]byte(fmt.Sprintf("%s\n", ssa.FmtObjMetadata(meta))))
	}

	return nil
}
