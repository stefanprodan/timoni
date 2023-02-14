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
	"cuelang.org/go/cue/cuecontext"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
)

var inspectValuesCmd = &cobra.Command{
	Use:   "values [MODULE]",
	Short: "Extract the default values from a module.",
	Example: `  # Print the default values of a local module
  timoni inspect values ./path/to/module
`,
	RunE: runInspectValuesCmd,
}

type inspectValuesFlags struct {
	module string
	pkg    string
}

var inspectValuesArgs inspectValuesFlags

func init() {
	inspectValuesCmd.Flags().StringVarP(&inspectValuesArgs.pkg, "package", "p", "main",
		"The name of the package containing the instance values and resources.")
	inspectCmd.AddCommand(inspectValuesCmd)
}

func runInspectValuesCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf(" module is required")
	}

	inspectValuesArgs.module = args[0]

	ctx := cuecontext.New()

	if _, err := os.Stat(inspectValuesArgs.module); err != nil {
		return fmt.Errorf("module not found at path %s", inspectValuesArgs.module)
	}

	tmpDir, err := os.MkdirTemp("", "timoni")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	modulePath := filepath.Join(tmpDir, "module")
	err = copyModule(inspectValuesArgs.module, modulePath)
	if err != nil {
		return err
	}

	builder := NewBuilder(ctx, "name", "namespace", modulePath, inspectValuesArgs.pkg)
	v, err := builder.GetDefaultValues()
	if err != nil {
		return err
	}
	cmd.OutOrStdout().Write([]byte(fmt.Sprintf("values: %v\n", v)))

	return err
}
