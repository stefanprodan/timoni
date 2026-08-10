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
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/logger"
	"github.com/stefanprodan/timoni/internal/oci"
)

var listModCmd = &cobra.Command{
	Use:     "list MODULE_URL",
	Args:    cobra.MaximumNArgs(1),
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

  # Check if a version is published using the JSON output
  timoni mod list oci://ghcr.io/org/modules/app -o json | \
	jq -e --arg v "1.0.0" 'any(.[]; .version == $v)'
`,
	RunE: listModCmdRun,
}

type listModFlags struct {
	creds      flags.Credentials
	withDigest bool
	output     string
}

var listModArgs listModFlags

func init() {
	listModCmd.Flags().Var(&listModArgs.creds, listModArgs.creds.Type(), listModArgs.creds.Description())
	listModCmd.Flags().BoolVar(&listModArgs.withDigest, "with-digest", true,
		"Resolve the digest of each version.")
	listModCmd.Flags().StringVarP(&listModArgs.output, "output", "o", "",
		"The format in which the versions should be printed, can be 'yaml' or 'json'.")
	modCmd.AddCommand(listModCmd)
}

func listModCmdRun(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("module URL is required")
	}
	ociURL := args[0]

	if err := validateOutputFormat(listModArgs.output, true); err != nil {
		return err
	}

	spin := logger.StartSpinner("fetching versions")
	defer spin.Stop()

	ctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	opts := oci.Options(ctx, listModArgs.creds.String(), rootArgs.registryInsecure)
	list, err := oci.ListModuleVersions(ociURL, listModArgs.withDigest, opts)
	if err != nil {
		return err
	}

	spin.Stop()

	if listModArgs.output == "" {
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

	if list == nil {
		list = []apiv1.ModuleReference{}
	}

	var marshalled []byte
	if listModArgs.output == "json" {
		marshalled, err = json.MarshalIndent(list, "", "  ")
		marshalled = append(marshalled, "\n"...)
	} else {
		marshalled, err = yaml.Marshal(list)
	}
	if err != nil {
		return err
	}

	_, err = cmd.OutOrStdout().Write(marshalled)
	return err
}
