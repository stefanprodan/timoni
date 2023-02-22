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

	oci "github.com/fluxcd/pkg/oci/client"
	"github.com/spf13/cobra"

	"github.com/stefanprodan/timoni/internal/flags"
)

var pullModCmd = &cobra.Command{
	Use:   "pull [MODULE URL]",
	Short: "Pull a module version from a container registry",
	Long: `The pull command downloads the module from a container registry and
extract its contents the specified directory.`,
	Example: `  # Pull a module version from GitHub Container Registry
  timoni mod pull oci://ghcr.io/org/manifests/app --version 1.0.0 \
	--output ./path/to/module \
	--creds timoni:$GITHUB_TOKEN
`,
	RunE: pullCmdRun,
}

type pullModFlags struct {
	version flags.Version
	output  string
	creds   flags.Credentials
}

var pullModArgs pullModFlags

func init() {
	pullModCmd.Flags().VarP(&pullModArgs.version, pullModArgs.version.Type(), pullModArgs.version.Shorthand(), pullModArgs.version.Description())
	pullModCmd.Flags().StringVarP(&pullModArgs.output, "output", "o", "",
		"The directory path where the module content should be extracted.")
	pullModCmd.Flags().Var(&pullModArgs.creds, pullModArgs.creds.Type(), pullModArgs.creds.Description())

	modCmd.AddCommand(pullModCmd)
}

func pullCmdRun(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("module URL is required")
	}
	ociURL := args[0]
	version := pullModArgs.version.String()

	if version == "" {
		return fmt.Errorf("module version is required")
	}

	if pullModArgs.output == "" {
		return fmt.Errorf("invalid output path %s", pullModArgs.output)
	}

	if fs, err := os.Stat(pullModArgs.output); err != nil || !fs.IsDir() {
		return fmt.Errorf("invalid output path %s", pullModArgs.output)
	}

	url, err := oci.ParseArtifactURL(ociURL + ":" + pullModArgs.version.String())
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	ociClient := oci.NewClient(nil)

	if pullModArgs.creds != "" {
		if err := ociClient.LoginWithCredentials(pullModArgs.creds.String()); err != nil {
			return fmt.Errorf("could not login with credentials: %w", err)
		}
	}

	if _, err := ociClient.Pull(ctx, url, pullModArgs.output); err != nil {
		return err
	}

	logger.Println("module extracted to", pullModArgs.output)

	return nil
}
