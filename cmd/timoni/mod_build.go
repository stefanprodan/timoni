/*
Copyright 2026 Stefan Prodan

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
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/logger"
	"github.com/stefanprodan/timoni/internal/oci"
)

var buildModCmd = &cobra.Command{
	Use:   "build MODULE",
	Short: "Build a module OCI artifact on the local filesystem",
	Args:  cobra.ExactArgs(1),
	RunE:  buildModCmdRun,
}

// buildModFlags contains local module build inputs.
type buildModFlags struct {
	version     string
	output      string
	format      string
	annotations []string
}

var buildModArgs buildModFlags

func init() {
	buildModCmd.Flags().StringVarP(&buildModArgs.version, "version", "v", "",
		"Module version e.g. '1.0.0'; overrides the 'org.opencontainers.image.version' annotation.")
	buildModCmd.Flags().StringVarP(&buildModArgs.output, "output", "o", "",
		"Path to the OCI archive or OCI image layout.")
	buildModCmd.Flags().StringVar(&buildModArgs.format, "format", string(oci.FormatArchive),
		"Output format, either 'oci-archive' or 'oci-layout'.")
	buildModCmd.Flags().StringArrayVarP(&buildModArgs.annotations, "annotation", "a", nil,
		"Set custom OCI annotations in the format '<key>=<value>'.")
	modCmd.AddCommand(buildModCmd)
}

// buildModCmdRun validates the module version, builds, and writes a local module artifact.
func buildModCmdRun(cmd *cobra.Command, args []string) error {
	if buildModArgs.output == "" {
		return fmt.Errorf("output path is required")
	}
	if err := flags.ValidateModuleVersion(buildModArgs.version); err != nil {
		return err
	}
	format := oci.LocalFormat(buildModArgs.format)
	if err := format.Validate(); err != nil {
		return err
	}
	module := args[0]
	if info, err := os.Stat(module); err != nil || !info.IsDir() {
		return fmt.Errorf("module not found at path %s", module)
	}

	annotations, err := oci.ParseAnnotations(buildModArgs.annotations)
	if err != nil {
		return err
	}
	// The required flag owns the manifest version annotation.
	annotations[apiv1.VersionAnnotation] = buildModArgs.version
	oci.AppendGitMetadata(module, annotations)
	ignorePaths, err := engine.ReadIgnoreFile(module)
	if err != nil {
		return fmt.Errorf("reading %s failed: %w", apiv1.IgnoreFile, err)
	}

	build, err := oci.BuildModuleImage(module, ignorePaths, annotations)
	if err != nil {
		return err
	}
	defer build.Close()
	if err := oci.WriteImage(build.Image, buildModArgs.output, format, []string{buildModArgs.version}); err != nil {
		return err
	}

	log := LoggerFrom(cmd.Context())
	log.Info(fmt.Sprintf("artifact: %s", logger.ColorizeSubject(filepath.Clean(buildModArgs.output))))
	log.Info(fmt.Sprintf("digest: %s", logger.ColorizeSubject(build.Digest.String())))
	return nil
}
