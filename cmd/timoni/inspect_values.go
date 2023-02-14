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
	"github.com/spf13/cobra"
	"os"
)

var inspectValuesCmd = &cobra.Command{
	Use:   "values [URL]",
	Short: "Extract the default values from a module",
	Example: `  # Print the default values of a local module
  timoni inspect values ./path/to/module

  # Print the default values of a remote module
  timoni inspect values oci://docker.io/org/module --version 1.0.0
`,
	RunE: runInspectValuesCmd,
}

type inspectValuesFlags struct {
	module  string
	version string
	pkg     string
	creds   string
}

var inspectValuesArgs inspectValuesFlags

func init() {
	inspectValuesCmd.Flags().StringVarP(&inspectValuesArgs.version, "version", "v", "",
		"version of the module.")
	inspectValuesCmd.Flags().StringVarP(&inspectValuesArgs.pkg, "package", "p", "main",
		"The name of the package containing the instance values and resources.")
	inspectValuesCmd.Flags().StringVar(&inspectValuesArgs.creds, "creds", "",
		"credentials for the container registry in the format <username>[:<password>]")
	inspectCmd.AddCommand(inspectValuesCmd)
}

func runInspectValuesCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf(" module is required")
	}

	inspectValuesArgs.module = args[0]

	ctx := cuecontext.New()

	tmpDir, err := os.MkdirTemp("", "timoni")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctxPull, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	fetcher := NewFetcher(ctxPull, inspectValuesArgs.module, inspectValuesArgs.version, tmpDir, inspectValuesArgs.creds)
	modulePath, err := fetcher.Fetch()
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
