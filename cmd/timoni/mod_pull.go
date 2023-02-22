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

var pullCmd = &cobra.Command{
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

type pullFlags struct {
	version flags.Version
	output  string
	creds   flags.Credentials
}

var pullArgs pullFlags

func init() {
	pullCmd.Flags().VarP(&pullArgs.version, pullArgs.version.Type(), pullArgs.version.Shorthand(), pullArgs.version.Description())
	pullCmd.Flags().StringVarP(&pullArgs.output, "output", "o", "",
		"The directory path where the module content should be extracted.")
	pullCmd.Flags().Var(&pullArgs.creds, pullArgs.creds.Type(), pullArgs.creds.Description())

	modCmd.AddCommand(pullCmd)
}

func pullCmdRun(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("module URL is required")
	}
	ociURL := args[0]
	version := pullArgs.version.String()

	if version == "" {
		return fmt.Errorf("module version is required")
	}

	if pullArgs.output == "" {
		return fmt.Errorf("invalid output path %s", pullArgs.output)
	}

	if fs, err := os.Stat(pullArgs.output); err != nil || !fs.IsDir() {
		return fmt.Errorf("invalid output path %s", pullArgs.output)
	}

	url, err := oci.ParseArtifactURL(ociURL + ":" + pullArgs.version.String())
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	ociClient := oci.NewClient(nil)

	if pullArgs.creds != "" {
		if err := ociClient.LoginWithCredentials(pullArgs.creds.String()); err != nil {
			return fmt.Errorf("could not login with credentials: %w", err)
		}
	}

	if _, err := ociClient.Pull(ctx, url, pullArgs.output); err != nil {
		return err
	}

	logger.Println("module extracted to", pullArgs.output)

	return nil
}
