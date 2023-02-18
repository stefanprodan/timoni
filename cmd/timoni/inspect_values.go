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
	"github.com/stefanprodan/timoni/internal/runtime"
)

var inspectValuesCmd = &cobra.Command{
	Use:   "values [INSTANCE NAME]",
	Short: "Print the values of an instance",
	Example: `  # Print the values
  timoni inspect values app

  # Export the values of an instance to a CUE file
  timoni -n default inspect values app > values.cue
`,
	RunE: runInspectValuesCmd,
}

type inspectValuesFlags struct {
	name string
}

var inspectValuesArgs inspectValuesFlags

func init() {
	inspectCmd.AddCommand(inspectValuesCmd)
}

func runInspectValuesCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("instance name is required")
	}
	inspectValuesArgs.name = args[0]

	sm, err := runtime.NewResourceManager(kubeconfigArgs)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	iStorage := runtime.NewStorageManager(sm)
	inst, err := iStorage.Get(ctx, inspectValuesArgs.name, *kubeconfigArgs.Namespace)
	if err != nil {
		return err
	}

	cmd.OutOrStdout().Write([]byte("values: " + inst.Values + "\n"))
	return nil
}
