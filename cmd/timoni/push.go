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
	"os"

	oci "github.com/fluxcd/pkg/oci/client"
	gcr "github.com/google/go-containerregistry/pkg/name"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

var pushCmd = &cobra.Command{
	Use:   "push [PATH] [MODULE URL]",
	Short: "Push a module to a container registry",
	Long: `The push command packages the module as an OCI artifact and pushes it to the
container registry using the version as the image tag.`,
	Example: `  # Push a module to Docker Hub using the credentials from '~/.docker/config.json'
  echo $DOCKER_PAT | docker login --username timoni --password-stdin
  timoni push ./path/to/module oci://docker.io/org/app \
	--source="$(git config --get remote.origin.url)" \
	--version=1.0.0

  # Push a module to GitHub Container Registry using a GitHub token
  timoni push ./path/to/module oci://ghcr.io/org/modules/app \
	--source="$(git config --get remote.origin.url)" \
	--version=1.0.0 \
	--creds timoni:$GITHUB_TOKEN
`,
	RunE: pushCmdRun,
}

type pushFlags struct {
	module      string
	source      string
	version     string
	creds       string
	ignorePaths []string
	output      string
}

var pushArgs pushFlags

func init() {
	pushCmd.Flags().StringVar(&pushArgs.source, "source", "",
		"the VCS address, e.g. the Git URL")
	pushCmd.Flags().StringVarP(&pushArgs.version, "version", "v", "",
		"the version in semver format e.g. '1.0.0'")
	pushCmd.Flags().StringVar(&pushArgs.creds, "creds", "",
		"credentials for the container registry in the format <username>[:<password>]")
	pushCmd.Flags().StringVarP(&pushArgs.output, "output", "o", "",
		"the format in which the artifact digest should be printed, can be 'json' or 'yaml'")

	rootCmd.AddCommand(pushCmd)
}

func pushCmdRun(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("module and URL are required")
	}
	pushArgs.module = args[0]
	ociURL := args[1]

	url, err := oci.ParseArtifactURL(ociURL + ":" + pushArgs.version)
	if err != nil {
		return err
	}

	if fs, err := os.Stat(pushArgs.module); err != nil || !fs.IsDir() {
		return fmt.Errorf("module not found at path %s", pushArgs.module)
	}

	path := pushArgs.module
	meta := oci.Metadata{
		Source:   pushArgs.source,
		Revision: pushArgs.version,
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	ociClient := oci.NewLocalClient()

	if pushArgs.creds != "" {
		if err := ociClient.LoginWithCredentials(pushArgs.creds); err != nil {
			return fmt.Errorf("could not login with credentials: %w", err)
		}
	}

	digestURL, err := ociClient.Push(ctx, url, path, meta, pushArgs.ignorePaths)
	if err != nil {
		return fmt.Errorf("pushing artifact failed: %w", err)
	}

	digest, err := gcr.NewDigest(digestURL)
	if err != nil {
		return fmt.Errorf("artifact digest parsing failed: %w", err)
	}

	tag, err := gcr.NewTag(url)
	if err != nil {
		return fmt.Errorf("artifact tag parsing failed: %w", err)
	}

	info := struct {
		URL        string `json:"url"`
		Repository string `json:"repository"`
		Tag        string `json:"tag"`
		Digest     string `json:"digest"`
	}{
		URL:        fmt.Sprintf("oci://%s", digestURL),
		Repository: digest.Repository.Name(),
		Tag:        tag.TagStr(),
		Digest:     digest.DigestStr(),
	}

	switch pushArgs.output {
	case "json":
		marshalled, err := json.MarshalIndent(&info, "", "  ")
		if err != nil {
			return fmt.Errorf("artifact digest JSON conversion failed: %w", err)
		}
		marshalled = append(marshalled, "\n"...)
		cmd.OutOrStdout().Write(marshalled)
	case "yaml":
		marshalled, err := yaml.Marshal(&info)
		if err != nil {
			return fmt.Errorf("artifact digest YAML conversion failed: %w", err)
		}
		cmd.OutOrStdout().Write(marshalled)
	default:
		cmd.OutOrStdout().Write([]byte(digestURL + "\n"))
	}

	return nil
}
