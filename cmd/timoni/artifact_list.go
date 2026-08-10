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

var listArtifactCmd = &cobra.Command{
	Use:     "list ARTIFACT_URL",
	Args:    cobra.MaximumNArgs(1),
	Aliases: []string{"ls"},
	Short:   "List the tags of an artifact",
	Long:    `The list command prints a table with the artifact tags and their digests.`,
	Example: `  # Print the tags and digests of an artifact
  timoni artifact ls oci://docker.io/org/app 

  # Print the tags without digests
  timoni artifact list oci://ghcr.io/org/bundles/app --with-digest=false

  # Print the tags and digests of an artifact stored in a private repository
  echo $DOCKER_TOKEN | timoni registry login docker.io -u timoni --password-stdin
  timoni artifact list oci://docker.io/org/app

  # Check if a tag is published using the JSON output
  timoni artifact list oci://ghcr.io/org/bundles/app -o json | \
	jq -e --arg t "latest" 'any(.[]; .tag == $t)'
`,
	RunE: listArtifactCmdRun,
}

type listArtifactFlags struct {
	creds      flags.Credentials
	withDigest bool
	output     string
}

var listArtifactArgs listArtifactFlags

func init() {
	listArtifactCmd.Flags().Var(&listArtifactArgs.creds, listArtifactArgs.creds.Type(), listArtifactArgs.creds.Description())
	listArtifactCmd.Flags().BoolVar(&listArtifactArgs.withDigest, "with-digest", true,
		"Resolve the digest of each version.")
	listArtifactCmd.Flags().StringVarP(&listArtifactArgs.output, "output", "o", "",
		"The format in which the tags should be printed, can be 'yaml' or 'json'.")
	artifactCmd.AddCommand(listArtifactCmd)
}

func listArtifactCmdRun(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("repository URL is required")
	}
	ociURL := args[0]

	if err := validateOutputFormat(listArtifactArgs.output, true); err != nil {
		return err
	}

	spin := logger.StartSpinner("fetching tags")
	defer spin.Stop()

	ctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	opts := oci.Options(ctx, listArtifactArgs.creds.String(), rootArgs.registryInsecure)
	list, err := oci.ListArtifactTags(ociURL, listArtifactArgs.withDigest, opts)
	if err != nil {
		return err
	}

	spin.Stop()

	if listArtifactArgs.output == "" {
		var rows [][]string
		for _, v := range list {
			row := []string{
				v.Tag,
				v.Digest,
			}
			rows = append(rows, row)
		}

		printTable(rootCmd.OutOrStdout(), []string{"tag", "digest"}, rows)

		return nil
	}

	if list == nil {
		list = []apiv1.ArtifactReference{}
	}

	var marshalled []byte
	if listArtifactArgs.output == "json" {
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
