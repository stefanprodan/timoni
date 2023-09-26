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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	oci "github.com/fluxcd/pkg/oci/client"
	"github.com/google/go-containerregistry/pkg/crane"
	gcr "github.com/google/go-containerregistry/pkg/name"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/signutil"
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
		"Tag of the artifact.")
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

	ociURL, err := oci.ParseRepositoryURL(args[0])
	if err != nil {
		return err
	}

	if len(pushArtifactArgs.tags) == 0 {
		return fmt.Errorf("at least one tag is required")
	}

	fi, err := os.Stat(pushArtifactArgs.path)
	if err != nil {
		return fmt.Errorf("file path not found %s", pushArtifactArgs.path)
	}
	path := pushArtifactArgs.path

	contentType := pushArtifactArgs.contentType
	if contentType == "" {
		return fmt.Errorf("content type is required")
	}

	if fi.IsDir() {
		ps, err := engine.ReadIgnoreFile(path)
		if err != nil {
			return fmt.Errorf("reading %s failed: %w", apiv1.IgnoreFile, err)
		}
		pushArtifactArgs.ignorePaths = append(pushArtifactArgs.ignorePaths, ps...)
	}

	log := LoggerFrom(cmd.Context())
	ociClient := oci.NewClient(nil)

	url := fmt.Sprintf("%s:%v", ociURL, pushArtifactArgs.tags[0])
	ref, err := gcr.ParseReference(url)
	if err != nil {
		return fmt.Errorf("'%s' invalid URL: %w", ociURL, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	// Try to determine the last Git commit timestamp
	ct := time.Now().UTC()
	created := ct.Format(time.RFC3339)
	gitCmd := exec.CommandContext(ctx, "git", "--no-pager", "log", "-1", `--format=%ct`)
	gitCmd.Dir = pushArtifactArgs.path
	if ts, err := gitCmd.Output(); err == nil && len(ts) > 1 {
		if i, err := strconv.ParseInt(strings.TrimSuffix(string(ts), "\n"), 10, 64); err == nil {
			d := time.Unix(i, 0)
			created = d.Format(time.RFC3339)
		}
	}

	annotations := map[string]string{}
	annotations["org.opencontainers.image.created"] = created
	for _, annotation := range pushArtifactArgs.annotations {
		kv := strings.Split(annotation, "=")
		if len(kv) != 2 {
			return fmt.Errorf("invalid annotation %s, must be in the format key=value", annotation)
		}
		annotations[kv[0]] = kv[1]
	}

	if pushArtifactArgs.creds != "" {
		if err := ociClient.LoginWithCredentials(pushArtifactArgs.creds.String()); err != nil {
			return fmt.Errorf("could not login with credentials: %w", err)
		}
	}

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "artifact.tgz")
	if err := ociClient.Build(tmpFile, path, pushArtifactArgs.ignorePaths); err != nil {
		return err
	}

	img := mutate.MediaType(empty.Image, types.OCIManifestSchema1)
	img = mutate.ConfigMediaType(img, apiv1.ConfigMediaType)
	img = mutate.Annotations(img, annotations).(gcrv1.Image)

	layer, err := tarball.LayerFromFile(tmpFile, tarball.WithMediaType(apiv1.ContentMediaType))
	if err != nil {
		return fmt.Errorf("creating content layer failed: %w", err)
	}

	img, err = mutate.Append(img, mutate.Addendum{
		Layer: layer,
		Annotations: map[string]string{
			apiv1.ContentTypeAnnotation: contentType,
		},
	})
	if err != nil {
		return fmt.Errorf("appending content to artifact failed: %w", err)
	}

	opts := append(ociClient.GetOptions(), crane.WithContext(ctx))
	if err := crane.Push(img, url, opts...); err != nil {
		return fmt.Errorf("pushing artifact failed: %w", err)
	}

	digest, err := img.Digest()
	if err != nil {
		return fmt.Errorf("parsing artifact digest failed: %w", err)
	}

	digestURL := ref.Context().Digest(digest.String()).String()
	if pushArtifactArgs.sign != "" {
		err = signutil.Sign(log, pushArtifactArgs.sign, digestURL, pushArtifactArgs.cosignKey)
		if err != nil {
			return err
		}
	}

	for i, tag := range pushArtifactArgs.tags {
		if i == 0 {
			continue
		}
		if err := crane.Tag(digestURL, tag, opts...); err != nil {
			return fmt.Errorf("tagging artifact with %s failed: %w", tag, err)
		}
	}

	log.Info(fmt.Sprintf("digest: %s", colorizeSubject(digestURL)))

	return nil
}
