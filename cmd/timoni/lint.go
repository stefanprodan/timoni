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
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var lintCmd = &cobra.Command{
	Use:   "lint [MODULE PATH]",
	Short: "Format and validate a local module",
	Long: `The list command formats the module's files with 'cue fmt' and
validates the cue definitions with 'cue vet -c'.
This command requires that the cue CLI binary is present in PATH.`,
	Example: `  # lint a local module
  timoni lint ./path/to/module
`,
	RunE: runLintCmd,
}

type lintFlags struct {
	module string
}

var lintArgs lintFlags

func init() {
	rootCmd.AddCommand(lintCmd)
}

func runLintCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("module path is required")
	}

	lintArgs.module = args[0]
	if _, err := os.Stat(lintArgs.module); err != nil {
		return fmt.Errorf("package not found at path %s", lintArgs.module)
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	logger.Println("formatting", lintArgs.module)
	if err := execCUE(ctx, lintArgs.module, "fmt", "./..."); err != nil {
		return err
	}

	logger.Println("vetting", lintArgs.module)
	if err := execCUE(ctx, lintArgs.module, "vet", "-c", "./..."); err != nil {
		return err
	}

	return nil
}

func execCUE(ctx context.Context, dir string, args ...string) error {
	var stdoutBuf, stderrBuf bytes.Buffer
	cueCmd := exec.CommandContext(ctx, "cue", args...)
	cueCmd.Dir = dir
	cueCmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cueCmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	return cueCmd.Run()
}
