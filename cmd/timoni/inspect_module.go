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

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/stefanprodan/timoni/internal/runtime"
)

var inspectModuleCmd = &cobra.Command{
	Use:   "module [INSTANCE NAME]",
	Short: "Print the module information of an instance",
	Example: `  # Print the module info
  timoni -n default inspect module app
`,
	RunE: runInspectModuleCmd,
}

type inspectModuleFlags struct {
	name string
}

var inspectModuleArgs inspectModuleFlags

func init() {
	inspectCmd.AddCommand(inspectModuleCmd)
}

func runInspectModuleCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("instance name is required")
	}
	inspectModuleArgs.name = args[0]

	sm, err := runtime.NewResourceManager(kubeconfigArgs)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	iStorage := runtime.NewStorageManager(sm)
	inst, err := iStorage.Get(ctx, inspectModuleArgs.name, *kubeconfigArgs.Namespace)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(inst.Module)
	if err != nil {
		return fmt.Errorf("failed to read module info, error: %w", err)
	}
	cmd.OutOrStdout().Write(data)
	return nil
}
