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
	"bytes"
	"context"
	"fmt"
	"os"

	oci "github.com/fluxcd/pkg/oci/client"
	"github.com/fluxcd/pkg/tar"
	"github.com/google/go-containerregistry/pkg/crane"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/signutil"
)

var pullArtifactCmd = &cobra.Command{
	Use:   "pull [URL]",
	Short: "Pull an artifact from a container registry",
	Long: `The pull command downloads the module from a container registry and
extract its contents the specified directory.`,
	Example: `  # Pull latest artifact and extract its contents to the current directory
  timoni artifact pull oci://docker.io/org/app:latest

  # Pull an artifact by tag from a private GHCR repository
  timoni artifact pull oci://ghcr.io/org/schemas/app:1.0.0 \
	--creds=timoni:$GITHUB_TOKEN \
	--output=./modules/my-app/cue.mod/pkg

  # Verify the Cosign signature and pull (the cosign binary must be present in PATH)
  timoni artifact pull oci://docker.io/org/app:latest \
	--verify=cosign \
	--cosign-key=/path/to/cosign.pub

  # Verify the Cosign keyless signature and pull (the cosign binary must be present in PATH)
  timoni artifact pull oci://ghcr.io/org/schemas/app:1.0.0 \
	--verify=cosign \
	--certificate-identity-regexp="^https://github.com/org/.*$" \
	--certificate-oidc-issuer=https://token.actions.githubusercontent.com \
	--output=./modules/my-app/cue.mod/pkg
`,
	RunE: pullArtifactCmdRun,
}

type pullArtifactFlags struct {
	output                      string
	contentType                 string
	creds                       flags.Credentials
	verify                      string
	cosignKey                   string
	certificateIdentity         string
	certificateIdentityRegexp   string
	certificateOidcIssuer       string
	certificateOidcIssuerRegexp string
}

var pullArtifactArgs pullArtifactFlags

func init() {
	pullArtifactCmd.Flags().StringVarP(&pullArtifactArgs.output, "output", "o", ".",
		"The directory path where the artifact content should be extracted.")
	pullArtifactCmd.Flags().Var(&pullArtifactArgs.creds, pullArtifactArgs.creds.Type(), pullArtifactArgs.creds.Description())
	pullArtifactCmd.Flags().StringVar(&pullArtifactArgs.contentType, "content-type", "",
		"Fetch the contents of the layers matching this type.")
	pullArtifactCmd.Flags().StringVar(&pullArtifactArgs.verify, "verify", "",
		"Verifies the signed artifact with the specified provider.")
	pullArtifactCmd.Flags().StringVar(&pullArtifactArgs.cosignKey, "cosign-key", "",
		"The Cosign public key for verifying the artifact.")
	pullArtifactCmd.Flags().StringVar(&pullArtifactArgs.certificateIdentity, "certificate-identity", "",
		"The identity expected in a valid Fulcio certificate for verifying the Cosign signature.\n"+
			"Valid values include email address, DNS names, IP addresses, and URIs.\n"+
			"Either --certificate-identity or --certificate-identity-regexp must be set for keyless flows.")
	pullArtifactCmd.Flags().StringVar(&pullArtifactArgs.certificateIdentityRegexp, "certificate-identity-regexp", "",
		"A regular expression alternative to --certificate-identity for verifying the Cosign signature.\n"+
			"Accepts the Go regular expression syntax described at https://golang.org/s/re2syntax.\n"+
			"Either --certificate-identity or --certificate-identity-regexp must be set for keyless flows.")
	pullArtifactCmd.Flags().StringVar(&pullArtifactArgs.certificateOidcIssuer, "certificate-oidc-issuer", "",
		"The OIDC issuer expected in a valid Fulcio certificate for verifying the Cosign signature,\n"+
			"e.g. https://token.actions.githubusercontent.com or https://oauth2.sigstore.dev/auth.\n"+
			"Either --certificate-oidc-issuer or --certificate-oidc-issuer-regexp must be set for keyless flows.")
	pullArtifactCmd.Flags().StringVar(&pullArtifactArgs.certificateOidcIssuerRegexp, "certificate-oidc-issuer-regexp", "",
		"A regular expression alternative to --certificate-oidc-issuer for verifying the Cosign signature.\n"+
			"Accepts the Go regular expression syntax described at https://golang.org/s/re2syntax.\n"+
			"Either --certificate-oidc-issuer or --certificate-oidc-issuer-regexp must be set for keyless flows.")

	artifactCmd.AddCommand(pullArtifactCmd)
}

func pullArtifactCmdRun(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("URL is required")
	}
	ociURL := args[0]
	contentType := pullArtifactArgs.contentType

	log := LoggerFrom(cmd.Context())

	if fs, err := os.Stat(pullArtifactArgs.output); err != nil || !fs.IsDir() {
		return fmt.Errorf("invalid output path %s", pullArtifactArgs.output)
	}

	url, err := oci.ParseArtifactURL(ociURL)
	if err != nil {
		return err
	}

	repoURL, err := oci.ParseRepositoryURL(ociURL)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	ociClient := oci.NewClient(nil)

	if pullArtifactArgs.creds != "" {
		if err := ociClient.LoginWithCredentials(pullArtifactArgs.creds.String()); err != nil {
			return fmt.Errorf("could not login with credentials: %w", err)
		}
	}

	if pullArtifactArgs.verify != "" {
		err = signutil.Verify(log, pullArtifactArgs.verify, url, pullArtifactArgs.cosignKey, pullArtifactArgs.certificateIdentity,
			pullArtifactArgs.certificateIdentityRegexp, pullArtifactArgs.certificateOidcIssuer, pullArtifactArgs.certificateOidcIssuerRegexp)
		if err != nil {
			return err
		}
	}

	spin := StartSpinner("pulling artifact")
	defer spin.Stop()

	opts := append(ociClient.GetOptions(), crane.WithContext(ctx))
	manifestJSON, err := crane.Manifest(url, opts...)
	if err != nil {
		return fmt.Errorf("pulling artifact manifest failed: %w", err)
	}

	manifest, err := gcrv1.ParseManifest(bytes.NewReader(manifestJSON))
	if err != nil {
		return fmt.Errorf("parsing artifact manifest failed: %w", err)
	}

	if manifest.Config.MediaType != apiv1.ConfigMediaType {
		return fmt.Errorf("unsupported artifact type '%s', must be '%s'",
			manifest.Config.MediaType, apiv1.ConfigMediaType)
	}

	var found bool
	for _, layer := range manifest.Layers {
		if layer.MediaType == apiv1.ContentMediaType {
			if contentType != "" && layer.Annotations[apiv1.ContentTypeAnnotation] != contentType {
				continue
			}
			found = true
			layerDigest := layer.Digest.String()
			blobURL := fmt.Sprintf("%s@%s", repoURL, layerDigest)
			layer, err := crane.PullLayer(blobURL, opts...)
			if err != nil {
				return fmt.Errorf("pulling artifact layer %s failed: %w", layerDigest, err)
			}

			blob, err := layer.Compressed()
			if err != nil {
				return fmt.Errorf("extracting artifact layer %s failed: %w", layerDigest, err)
			}

			if err = tar.Untar(blob, pullArtifactArgs.output, tar.WithMaxUntarSize(-1)); err != nil {
				return fmt.Errorf("extracting artifact layer %s failed: %w", layerDigest, err)
			}
		}
	}

	if !found {
		return fmt.Errorf("no layer found in artifact")
	}

	spin.Stop()
	log.Info(fmt.Sprintf("extracted: %s", colorizeSubject(pullArtifactArgs.output)))

	return nil
}
