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

	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/oci"
)

var listModCmd = &cobra.Command{
	Use:     "list [MODULE URL]",
	Aliases: []string{"ls"},
	Short:   "List the versions of a module",
	Long:    `The list command prints a table with the module versions and their digests.`,
	Example: `  # Print the versions and digests of a module
  timoni mod list oci://docker.io/org/app 

  # Print the versions without digests
  timoni mod list oci://docker.io/org/app --with-digest=false

  # Print the versions of a module from GitHub Container Registry
  timoni mod list oci://ghcr.io/org/manifests/app \
	--creds timoni:$GITHUB_TOKEN
`,
	RunE: listModCmdRun,
}

type listModFlags struct {
	creds      flags.Credentials
	withDigest bool
}

var listModArgs listModFlags

func init() {
	listModCmd.Flags().Var(&listModArgs.creds, listModArgs.creds.Type(), listModArgs.creds.Description())
	listModCmd.Flags().BoolVar(&listModArgs.withDigest, "with-digest", true,
		"Resolve the digest of each version.")
	modCmd.AddCommand(listModCmd)
}

func listModCmdRun(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("module URL is required")
	}
	ociURL := args[0]

	spin := StartSpinner("fetching versions")
	defer spin.Stop()

	ctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	opts := oci.Options(ctx, listModArgs.creds.String())
	list, err := oci.ListModuleVersions(ociURL, listModArgs.withDigest, opts)
	if err != nil {
		return err
	}

	spin.Stop()
	var rows [][]string
	for _, v := range list {
		row := []string{
			v.Version,
			v.Digest,
		}
		rows = append(rows, row)
	}

	printTable(rootCmd.OutOrStdout(), []string{"version", "digest"}, rows)

	return nil
}
