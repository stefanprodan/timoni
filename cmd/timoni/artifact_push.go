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
	"os"

	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/oci"
)

var pushArtifactCmd = &cobra.Command{
	Use:   "push [REPOSITORY URL]",
	Short: "Push a directory contents to a container registry",
	Long: `The push command packages a directory contents as an OCI artifact and pushes
it to the container registry. If the directory contains a timoni.ignore file,
the ignore rules will be used to exclude files from the artifact.`,
	Example: `  # Push the current dir contents to Docker Hub using the credentials from '~/.docker/config.json'
  echo $DOCKER_PAT | docker login --username timoni --password-stdin
  timoni artifact push oci://docker.io/org/app -t latest -f .

 # Push a dir contents to GitHub Container Registry using a GitHub token
  timoni artifact push oci://ghcr.io/org/schemas/app -f ./path/to/bundles \
	--creds=timoni:$GITHUB_TOKEN \
	--tag="$(git rev-parse --short HEAD)" \
	--tag=latest \
	--annotation="org.opencontainers.image.source=$(git config --get remote.origin.url)" \
	--annotation="org.opencontainers.image.revision=$(git rev-parse HEAD)' \
	--content-type="timoni.sh/bundles"

  # Push and sign with Cosign (the cosign binary must be present in PATH)
  echo $GITHUB_TOKEN | timoni registry login ghcr.io -u timoni --password-stdin
  export COSIGN_PASSWORD=password
  timoni artifact push oci://ghcr.io/org/schemas/app \
	-f=/path/to/schemas \
	--tag=1.0.0 \
	--sign=cosign \
	--cosign-key=/path/to/cosign.key
`,
	RunE: pushArtifactCmdRun,
}

type pushArtifactFlags struct {
	path        string
	creds       flags.Credentials
	ignorePaths []string
	tags        []string
	annotations []string
	contentType string
	sign        string
	cosignKey   string
}

var pushArtifactArgs pushArtifactFlags

func init() {
	pushArtifactCmd.Flags().StringVarP(&pushArtifactArgs.path, "filepath", "f", ".",
		"Path to local file or directory.")
	pushArtifactCmd.Flags().Var(&pushArtifactArgs.creds, pushArtifactArgs.creds.Type(), pushArtifactArgs.creds.Description())
	pushArtifactCmd.Flags().StringArrayVarP(&pushArtifactArgs.tags, "tag", "t", nil,
		"TagArtifact of the artifact.")
	pushArtifactCmd.Flags().StringArrayVarP(&pushArtifactArgs.annotations, "annotation", "a", nil,
		"Annotation in the format '<key>=<value>'.")
	pushArtifactCmd.Flags().StringVar(&pushArtifactArgs.contentType, "content-type", "generic",
		"The content type of this artifact.")
	pushArtifactCmd.Flags().StringVar(&pushArtifactArgs.sign, "sign", "",
		"Signs the module with the specified provider.")
	pushArtifactCmd.Flags().StringVar(&pushArtifactArgs.cosignKey, "cosign-key", "",
		"The Cosign private key for signing the module.")

	artifactCmd.AddCommand(pushArtifactCmd)
}

func pushArtifactCmdRun(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("repository URL is required")
	}

	if len(pushArtifactArgs.tags) == 0 {
		return fmt.Errorf("at least one tag is required")
	}

	fi, err := os.Stat(pushArtifactArgs.path)
	if err != nil {
		return fmt.Errorf("file path not found %s", pushArtifactArgs.path)
	}

	contentType := pushArtifactArgs.contentType
	if contentType == "" {
		return fmt.Errorf("content type is required")
	}

	if fi.IsDir() {
		ps, err := engine.ReadIgnoreFile(pushArtifactArgs.path)
		if err != nil {
			return fmt.Errorf("reading %s failed: %w", apiv1.IgnoreFile, err)
		}
		pushArtifactArgs.ignorePaths = append(pushArtifactArgs.ignorePaths, ps...)
	}

	log := LoggerFrom(cmd.Context())
	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	annotations, err := oci.ParseAnnotations(pushArtifactArgs.annotations)
	if err != nil {
		return err
	}
	oci.AppendCreated(ctx, pushArtifactArgs.path, annotations)

	spin := StartSpinner("pushing artifact")
	defer spin.Stop()

	opts := oci.Options(ctx, pushArtifactArgs.creds.String())
	ociURL := fmt.Sprintf("%s:%s", args[0], pushArtifactArgs.tags[0])
	digestURL, err := oci.PushArtifact(ociURL,
		pushArtifactArgs.path,
		pushArtifactArgs.ignorePaths,
		pushArtifactArgs.contentType,
		annotations,
		opts)
	if err != nil {
		return err
	}

	for i, tag := range pushArtifactArgs.tags {
		if i == 0 {
			continue
		}
		if err := oci.TagArtifact(digestURL, tag, opts); err != nil {
			return fmt.Errorf("tagging artifact with %s failed: %w", tag, err)
		}
	}

	spin.Stop()
	if pushArtifactArgs.sign != "" {
		err = oci.SignArtifact(log, pushArtifactArgs.sign, digestURL, pushArtifactArgs.cosignKey)
		if err != nil {
			return err
		}
	}

	log.Info(fmt.Sprintf("digest: %s", colorizeSubject(digestURL)))

	return nil
}
