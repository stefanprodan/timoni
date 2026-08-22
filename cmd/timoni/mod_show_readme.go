/*
Copyright 2026 Stefan Prodan
SPDX-License-Identifier: Apache-2.0

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
	"path/filepath"

	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine/fetcher"
	"github.com/stefanprodan/timoni/internal/flags"
)

var readmeShowModCmd = &cobra.Command{
	Use:   "readme [MODULE PATH | MODULE URL]",
	Args:  cobra.MaximumNArgs(1),
	Short: "Output the README.md of a module",
	Long: `The readme command prints the README.md file of a module to stdout.
The module can be a local directory or an OCI artifact.`,
	Example: `  # print the readme of a module in the current directory
  timoni mod show readme

  # print the readme of a module published to a container registry
  timoni mod show readme oci://docker.io/org/app -v 1.0.0
`,
	RunE: runReadmeShowModCmd,
}

type readmeModFlags struct {
	path    string
	version flags.Version
	creds   flags.Credentials
}

var readmeShowModArgs readmeModFlags

func init() {
	readmeShowModCmd.Flags().VarP(&readmeShowModArgs.version, readmeShowModArgs.version.Type(), readmeShowModArgs.version.Shorthand(), readmeShowModArgs.version.Description())
	readmeShowModCmd.Flags().Var(&readmeShowModArgs.creds, readmeShowModArgs.creds.Type(), readmeShowModArgs.creds.Description())
	showModCmd.AddCommand(readmeShowModCmd)
}

func runReadmeShowModCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		readmeShowModArgs.path = "."
	} else {
		readmeShowModArgs.path = args[0]
	}

	version := readmeShowModArgs.version.String()
	if version == "" {
		version = apiv1.LatestVersion
	}

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctxPull, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	f, err := fetcher.New(ctxPull, fetcher.Options{
		Source:       readmeShowModArgs.path,
		Version:      version,
		Destination:  tmpDir,
		CacheDir:     rootArgs.cacheDir,
		Creds:        readmeShowModArgs.creds.String(),
		Insecure:     rootArgs.registryInsecure,
		DefaultLocal: true,
	})
	if err != nil {
		return err
	}

	if _, err := f.Fetch(); err != nil {
		return err
	}

	readme, err := os.ReadFile(filepath.Join(f.GetModuleRoot(), "README.md"))
	if err != nil {
		return fmt.Errorf("reading README.md from module %s failed: %w", readmeShowModArgs.path, err)
	}

	_, err = rootCmd.OutOrStdout().Write(readme)
	return err
}
