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
	Long: `The list command prints a table with the artifact tags and their digests.
By default, the last 100 tags in descending lexical order are listed,
use '--limit 0' to list all tags.`,
	Example: `  # Print the last 100 tags and their digests
  timoni artifact ls oci://docker.io/org/app

  # Print the last 10 tags
  timoni artifact list oci://docker.io/org/app --limit 10

  # Print all the tags without digests
  timoni artifact list oci://ghcr.io/org/bundles/app --limit 0 --with-digest=false

  # Print the tags matching a regular expression
  timoni artifact list oci://docker.io/org/app --filter-regex '^1\.'

  # Print the tags matching a semver range
  timoni artifact list oci://docker.io/org/app --filter-semver '>=1.0.0 <2.0.0'

  # Print the tags and digests of an artifact stored in a private repository
  echo $DOCKER_TOKEN | timoni registry login docker.io -u timoni --password-stdin
  timoni artifact list oci://docker.io/org/app

  # Check if a tag is published using the JSON output
  timoni artifact list oci://ghcr.io/org/bundles/app --limit 0 -o json | \
	jq -e --arg t "latest" 'any(.[]; .tag == $t)'
`,
	RunE: listArtifactCmdRun,
}

type listArtifactFlags struct {
	creds        flags.Credentials
	withDigest   bool
	limit        int
	output       string
	filterRegex  string
	filterSemver string
}

var listArtifactArgs listArtifactFlags

func init() {
	listArtifactCmd.Flags().Var(&listArtifactArgs.creds, listArtifactArgs.creds.Type(), listArtifactArgs.creds.Description())
	listArtifactCmd.Flags().BoolVar(&listArtifactArgs.withDigest, "with-digest", true,
		"Resolve the digest of each tag.")
	listArtifactCmd.Flags().IntVar(&listArtifactArgs.limit, "limit", 100,
		"Limit the number of tags listed in descending lexical order (0 for all).")
	listArtifactCmd.Flags().StringVarP(&listArtifactArgs.output, "output", "o", "",
		"The format in which the tags should be printed, can be 'yaml' or 'json'.")
	listArtifactCmd.Flags().StringVar(&listArtifactArgs.filterRegex, "filter-regex", "",
		"Filter tags returned from the OCI repository using regular expressions.")
	listArtifactCmd.Flags().StringVar(&listArtifactArgs.filterSemver, "filter-semver", "",
		"Filter tags returned from the OCI repository using semantic version ranges.")
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

	if listArtifactArgs.limit < 0 {
		return fmt.Errorf("--limit must be greater than or equal to 0")
	}

	spin := logger.StartSpinner("fetching tags")
	defer spin.Stop()

	ctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	opts := oci.Options(ctx, listArtifactArgs.creds.String(), rootArgs.registryInsecure)
	list, total, err := oci.ListArtifactTags(ctx, ociURL, oci.ListArtifactOptions{
		WithDigest:   listArtifactArgs.withDigest,
		FilterRegex:  listArtifactArgs.filterRegex,
		FilterSemver: listArtifactArgs.filterSemver,
		Limit:        listArtifactArgs.limit,
	}, opts)
	if err != nil {
		return err
	}

	spin.Stop()

	if listArtifactArgs.limit > 0 && total > listArtifactArgs.limit {
		log := LoggerFrom(cmd.Context())
		log.Info(fmt.Sprintf("showing %d of %d tags, use --limit 0 for all",
			listArtifactArgs.limit, total))
	}

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
