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

var tagArtifactCmd = &cobra.Command{
	Use:   "tag [ARTIFACT URL]",
	Short: "Tag an OCI artifact in the upstream registry",
	Long:  `The tag command allows adding tags to an existing artifact.`,
	Example: `  # Tag an existing artifact with a new tags
  echo $DOCKER_PAT | docker login --username timoni --password-stdin
  timoni artifact tag oci://docker.io/org/app:1.0.0 -t 1.0 -t latest
`,
	RunE: tagArtifactCmdRun,
}

type tagArtifactFlags struct {
	creds flags.Credentials
	tags  []string
}

var tagArtifactArgs tagArtifactFlags

func init() {
	tagArtifactCmd.Flags().Var(&tagArtifactArgs.creds, tagArtifactArgs.creds.Type(), tagArtifactArgs.creds.Description())
	tagArtifactCmd.Flags().StringArrayVarP(&tagArtifactArgs.tags, "tag", "t", nil,
		"Tag of the artifact.")

	artifactCmd.AddCommand(tagArtifactCmd)
}

func tagArtifactCmdRun(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("artifact URL is required")
	}
	ociURL := args[0]

	if len(tagArtifactArgs.tags) == 0 {
		return fmt.Errorf("at least one tag is required")
	}

	spin := StartSpinner("tagging artifact")
	defer spin.Stop()

	log := LoggerFrom(cmd.Context())
	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	opts := oci.Options(ctx, tagArtifactArgs.creds.String())

	for _, tag := range tagArtifactArgs.tags {
		if err := oci.TagArtifact(ociURL, tag, opts); err != nil {
			spin.Stop()
			return fmt.Errorf("tagging artifact with %s failed: %w", tag, err)
		}
	}

	spin.Stop()

	baseURL, err := oci.ParseRepositoryURL(ociURL)
	if err != nil {
		return err
	}

	for _, tag := range tagArtifactArgs.tags {
		log.Info(fmt.Sprintf("tagged: %s", colorizeSubject(fmt.Sprintf("%s:%s", baseURL, tag))))
	}

	return nil
}
