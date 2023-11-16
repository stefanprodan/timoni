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

	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/oci"
)

var pullArtifactCmd = &cobra.Command{
	Use:   "pull [ARTIFACT URL]",
	Short: "Pull an artifact from a container registry",
	Long: `The pull command downloads an artifact with the application/vnd.timoni media type
from a container registry and extract the selected layers to the specified directory.`,
	Example: `  # Pull latest artifact and extract its contents to the current directory
  timoni artifact pull oci://docker.io/org/app:latest

  # Pull an artifact by tag from a private GHCR repository
  echo $GITHUB_TOKEN | timoni registry login ghcr.io -u timoni --password-stdin
  timoni artifact pull oci://ghcr.io/org/schemas/app:1.0.0 \
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

	log := LoggerFrom(cmd.Context())

	if err := os.MkdirAll(pullArtifactArgs.output, os.ModePerm); err != nil {
		return fmt.Errorf("invalid output path %s: %w", pullArtifactArgs.output, err)
	}

	if pullArtifactArgs.verify != "" {
		err := oci.VerifyArtifact(log,
			pullArtifactArgs.verify,
			ociURL,
			pullArtifactArgs.cosignKey,
			pullArtifactArgs.certificateIdentity,
			pullArtifactArgs.certificateIdentityRegexp,
			pullArtifactArgs.certificateOidcIssuer,
			pullArtifactArgs.certificateOidcIssuerRegexp)
		if err != nil {
			return err
		}
	}

	spin := StartSpinner("pulling artifact")
	defer spin.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	opts := oci.Options(ctx, pullArtifactArgs.creds.String())
	err := oci.PullArtifact(ociURL, pullArtifactArgs.output, pullArtifactArgs.contentType, opts)
	if err != nil {
		return err
	}

	spin.Stop()
	log.Info(fmt.Sprintf("extracted: %s", colorizeSubject(pullArtifactArgs.output)))

	return nil
}
