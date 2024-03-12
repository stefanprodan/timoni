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

	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/logger"
	"github.com/stefanprodan/timoni/internal/oci"
)

var vendorK8sCmd = &cobra.Command{
	Use:   "k8s [MODULE PATH]",
	Short: "Vendor Kubernetes API CUE schemas",
	Example: `  # Vendor CUE schemas generated from the latest Kubernetes GA APIs
  timoni mod vendor k8s

  # Vendor CUE schemas generated from a specific version of Kubernetes GA APIs
  timoni mod vendor k8s -v 1.28
`,
	RunE: runVendorK8sCmd,
}

type vendorK8sFlags struct {
	modRoot string
	version string
}

var vendorK8sArgs vendorK8sFlags

func init() {
	vendorK8sCmd.Flags().StringVarP(&vendorK8sArgs.version, "version", "v", "latest",
		"The Kubernetes minor version e.g. 1.28.")

	modVendorCmd.AddCommand(vendorK8sCmd)
}

const k8sSchemaURL = "oci://ghcr.io/stefanprodan/timoni/kubernetes-schema"

func runVendorK8sCmd(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		vendorK8sArgs.modRoot = args[0]
	}

	log := logger.LoggerFrom(cmd.Context())

	// Make sure we're importing into a CUE module.
	cueModDir := path.Join(vendorK8sArgs.modRoot, "cue.mod")
	if fs, err := os.Stat(cueModDir); err != nil || !fs.IsDir() {
		return fmt.Errorf("cue.mod not found in the module path %s", vendorK8sArgs.modRoot)
	}

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	ociURL := fmt.Sprintf("%s:%s", k8sSchemaURL, vendorK8sArgs.version)
	if ver := vendorK8sArgs.version; ver != "latest" && !strings.HasPrefix(ver, "v") {
		ociURL = fmt.Sprintf("%s:v%s", k8sSchemaURL, ver)
	}

	spin := logger.StartSpinner(fmt.Sprintf("importing schemas from %s", ociURL))
	defer spin.Stop()

	opts := oci.Options(ctx, "", rootArgs.registryInsecure)
	err := oci.PullArtifact(ociURL, path.Join(cueModDir, "gen"), apiv1.CueModGenContentType, opts)
	if err != nil {
		return err
	}

	spin.Stop()
	log.Info(fmt.Sprintf("schemas vendored: %s", logger.ColorizeSubject(path.Join(cueModDir, "gen", "k8s.io", "api"))))

	return nil
}
