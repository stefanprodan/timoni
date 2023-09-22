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
	"path"
	"strings"

	oci "github.com/fluxcd/pkg/oci/client"
	"github.com/spf13/cobra"
)

var importK8sCmd = &cobra.Command{
	Use:   "k8s [MODULE PATH]",
	Short: "Import Kubernetes API CUE schemas",
	Example: `  # Import the latest Kubernetes API schemas
  timoni mod import k8s

  # Import s specific minor version of the Kubernetes API schemas
  timoni mod import k8s -v 1.28
`,
	RunE: runimportK8sCmd,
}

type importK8sFlags struct {
	modRoot string
	version string
}

var importK8sArgs importK8sFlags

func init() {
	importK8sCmd.Flags().StringVarP(&importK8sArgs.version, "version", "v", "latest",
		"The Kubernetes minor version e.g. 1.28.")

	modImportCmd.AddCommand(importK8sCmd)
}

const k8sSchemaURL = "ghcr.io/stefanprodan/timoni/kubernetes-schema"

func runimportK8sCmd(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		importK8sArgs.modRoot = args[0]
	}

	log := LoggerFrom(cmd.Context())

	// Make sure we're importing into a CUE module.
	cueModDir := path.Join(importK8sArgs.modRoot, "cue.mod")
	if fs, err := os.Stat(cueModDir); err != nil || !fs.IsDir() {
		return fmt.Errorf("cue.mod not found in the module path %s", importK8sArgs.modRoot)
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	ociClient := oci.NewClient(nil)

	url := fmt.Sprintf("%s:%s", k8sSchemaURL, importK8sArgs.version)
	if ver := importK8sArgs.version; ver != "latest" && !strings.HasPrefix(ver, "v") {
		url = fmt.Sprintf("%s:v%s", k8sSchemaURL, ver)
	}

	spin := StartSpinner(fmt.Sprintf("importing schemas from %s", url))
	_, err := ociClient.Pull(ctx, url, path.Join(cueModDir, "gen"))
	spin.Stop()
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("schemas imported: %s", colorizeSubject(path.Join(cueModDir, "gen", "k8s.io", "api"))))

	return nil
}
