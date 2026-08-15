/*
Copyright 2026 Stefan Prodan

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

	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/logger"
	"github.com/stefanprodan/timoni/internal/oci"
)

var digestArtifactCmd = &cobra.Command{
	Use:   "digest ARTIFACT_URL",
	Args:  cobra.MaximumNArgs(1),
	Short: "Resolve the digest of an artifact",
	Long: `The digest command resolves the digest of an artifact tag.
When the URL contains no tag, the 'latest' tag is used.`,
	Example: `  # Print the digest of the latest tag
  timoni artifact digest oci://docker.io/org/app

  # Print the digest of a specific tag
  timoni artifact digest oci://docker.io/org/app:1.0.0

  # Print the digest of an artifact stored in a private repository
  echo $DOCKER_TOKEN | timoni registry login docker.io -u timoni --password-stdin
  timoni artifact digest oci://docker.io/org/app

  # Extract the digest using the JSON output
  timoni artifact digest oci://ghcr.io/org/bundles/app -o json | jq -r '.digest'
`,
	RunE: digestArtifactCmdRun,
}

type digestArtifactFlags struct {
	creds  flags.Credentials
	output string
}

var digestArtifactArgs digestArtifactFlags

func init() {
	digestArtifactCmd.Flags().Var(&digestArtifactArgs.creds, digestArtifactArgs.creds.Type(), digestArtifactArgs.creds.Description())
	digestArtifactCmd.Flags().StringVarP(&digestArtifactArgs.output, "output", "o", "",
		"The format in which the digest should be printed, can be 'yaml' or 'json'.")

	artifactCmd.AddCommand(digestArtifactCmd)
}

// digestArtifactCmdRun resolves the digest of an OCI artifact tag
// and prints the artifact reference in the specified format.
func digestArtifactCmdRun(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("artifact URL is required")
	}
	ociURL := args[0]

	if err := validateOutputFormat(digestArtifactArgs.output, true); err != nil {
		return err
	}

	spin := logger.StartSpinner("resolving digest")
	defer spin.Stop()

	log := LoggerFrom(cmd.Context())
	ctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	opts := oci.Options(ctx, digestArtifactArgs.creds.String(), rootArgs.registryInsecure)
	ref, err := oci.GetArtifactDigest(ociURL, opts)
	if err != nil {
		return err
	}

	spin.Stop()

	switch digestArtifactArgs.output {
	case "json":
		marshalled, err := json.MarshalIndent(&ref, "", "  ")
		if err != nil {
			return fmt.Errorf("artifact digest JSON conversion failed: %w", err)
		}
		marshalled = append(marshalled, "\n"...)
		_, err = cmd.OutOrStdout().Write(marshalled)
		return err
	case "yaml":
		marshalled, err := yaml.Marshal(&ref)
		if err != nil {
			return fmt.Errorf("artifact digest YAML conversion failed: %w", err)
		}
		_, err = cmd.OutOrStdout().Write(marshalled)
		return err
	default:
		url := ref.Repository
		if ref.Tag != "" {
			url = fmt.Sprintf("%s:%s", ref.Repository, ref.Tag)
		}
		log.Info(fmt.Sprintf("artifact: %s", logger.ColorizeSubject(url)))
		log.Info(fmt.Sprintf("digest: %s", logger.ColorizeSubject(ref.Digest)))
	}

	return nil
}
