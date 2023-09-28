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
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/oci"
)

var pullModCmd = &cobra.Command{
	Use:   "pull [MODULE URL]",
	Short: "Pull a module version from a container registry",
	Long: `The pull command downloads the module from a container registry and
extract its contents the specified directory.`,
	Example: `  # Pull the latest stable version of a module
  echo $DOCKER_TOKEN | timoni registry login docker.io -u timoni --password-stdin
  timoni mod pull oci://docker.io/org/app \
	--output ./path/to/module

  # Pull a specific module version from GitHub Container Registry
  timoni mod pull oci://ghcr.io/org/manifests/app --version 1.0.0 \
	--output ./path/to/module \
	--creds timoni:$GITHUB_TOKEN
`,
	RunE: pullCmdRun,
}

type pullModFlags struct {
	version                     flags.Version
	output                      string
	creds                       flags.Credentials
	verify                      string
	cosignKey                   string
	certificateIdentity         string
	certificateIdentityRegexp   string
	certificateOidcIssuer       string
	certificateOidcIssuerRegexp string
}

var pullModArgs pullModFlags

func init() {
	pullModCmd.Flags().VarP(&pullModArgs.version, pullModArgs.version.Type(), pullModArgs.version.Shorthand(), pullModArgs.version.Description())
	pullModCmd.Flags().StringVarP(&pullModArgs.output, "output", "o", "",
		"The directory path where the module content should be extracted.")
	pullModCmd.Flags().Var(&pullModArgs.creds, pullModArgs.creds.Type(), pullModArgs.creds.Description())
	pullModCmd.Flags().StringVar(&pullModArgs.verify, "verify", "",
		"Verifies the signed module with the specified provvider.")
	pullModCmd.Flags().StringVar(&pullModArgs.cosignKey, "cosign-key", "",
		"The Cosign public key for verifying the module.")
	pullModCmd.Flags().StringVar(&pullModArgs.certificateIdentity, "certificate-identity", "",
		"The identity expected in a valid Fulcio certificate for verifying the Cosign signature.\n"+
			"Valid values include email address, DNS names, IP addresses, and URIs.\n"+
			"Either --certificate-identity or --certificate-identity-regexp must be set for keyless flows.")
	pullModCmd.Flags().StringVar(&pullModArgs.certificateIdentityRegexp, "certificate-identity-regexp", "",
		"A regular expression alternative to --certificate-identity for verifying the Cosign signature.\n"+
			"Accepts the Go regular expression syntax described at https://golang.org/s/re2syntax.\n"+
			"Either --certificate-identity or --certificate-identity-regexp must be set for keyless flows.")
	pullModCmd.Flags().StringVar(&pullModArgs.certificateOidcIssuer, "certificate-oidc-issuer", "",
		"The OIDC issuer expected in a valid Fulcio certificate for verifying the Cosign signature,\n"+
			"e.g. https://token.actions.githubusercontent.com or https://oauth2.sigstore.dev/auth.\n"+
			"Either --certificate-oidc-issuer or --certificate-oidc-issuer-regexp must be set for keyless flows.")
	pullModCmd.Flags().StringVar(&pullModArgs.certificateOidcIssuerRegexp, "certificate-oidc-issuer-regexp", "",
		"A regular expression alternative to --certificate-oidc-issuer for verifying the Cosign signature.\n"+
			"Accepts the Go regular expression syntax described at https://golang.org/s/re2syntax.\n"+
			"Either --certificate-oidc-issuer or --certificate-oidc-issuer-regexp must be set for keyless flows.")

	modCmd.AddCommand(pullModCmd)
}

func pullCmdRun(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("module URL is required")
	}

	version := pullModArgs.version.String()
	if version == "" {
		version = apiv1.LatestVersion
	}
	ociURL := fmt.Sprintf("%s:%s", args[0], version)

	if pullModArgs.output == "" {
		return fmt.Errorf("invalid output path %s", pullModArgs.output)
	}

	if fs, err := os.Stat(pullModArgs.output); err != nil || !fs.IsDir() {
		return fmt.Errorf("invalid output path %s", pullModArgs.output)
	}

	log := LoggerFrom(cmd.Context())

	if pullModArgs.verify != "" {
		err := oci.VerifyArtifact(log,
			pullModArgs.verify,
			ociURL,
			pullModArgs.cosignKey,
			pullModArgs.certificateIdentity,
			pullModArgs.certificateIdentityRegexp,
			pullModArgs.certificateOidcIssuer,
			pullModArgs.certificateOidcIssuerRegexp)
		if err != nil {
			return err
		}
	}

	spin := StartSpinner("pulling module")
	defer spin.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	opts := oci.Options(ctx, pullModArgs.creds.String())
	err := oci.PullArtifact(ociURL, pullModArgs.output, apiv1.AnyContentType, opts)
	if err != nil {
		return err
	}

	spin.Stop()
	log.Info(fmt.Sprintf("module extracted to %s", pullModArgs.output))

	return nil
}
