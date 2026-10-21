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
	"github.com/stefanprodan/timoni/internal/logger"
	"github.com/stefanprodan/timoni/internal/oci"
)

var buildArtifactCmd = &cobra.Command{
	Use:   "build",
	Short: "Build an OCI artifact on the local filesystem",
	Args:  cobra.NoArgs,
	RunE:  buildArtifactCmdRun,
}

// buildArtifactFlags contains local artifact build inputs.
type buildArtifactFlags struct {
	path            string
	output          string
	format          string
	tags            []string
	annotations     []string
	contentType     string
	resolveSymlinks bool
}

var buildArtifactArgs buildArtifactFlags

func init() {
	buildArtifactCmd.Flags().StringVarP(&buildArtifactArgs.path, "filepath", "f", ".",
		"Path to a local file or directory.")
	buildArtifactCmd.Flags().StringVarP(&buildArtifactArgs.output, "output", "o", "",
		"Path to the OCI archive or OCI image layout.")
	buildArtifactCmd.Flags().StringVar(&buildArtifactArgs.format, "format", string(oci.FormatArchive),
		"Output format, either 'oci-archive' or 'oci-layout'.")
	buildArtifactCmd.Flags().StringArrayVarP(&buildArtifactArgs.tags, "tag", "t", nil,
		"Local descriptor reference name.")
	buildArtifactCmd.Flags().StringArrayVarP(&buildArtifactArgs.annotations, "annotation", "a", nil,
		"Annotation in the format '<key>=<value>'.")
	buildArtifactCmd.Flags().StringVar(&buildArtifactArgs.contentType, "content-type", "generic",
		"The content type of this artifact.")
	buildArtifactCmd.Flags().BoolVar(&buildArtifactArgs.resolveSymlinks, "resolve-symlinks", false,
		"Resolve symbolic links and package their targets as regular files and directories.")
	artifactCmd.AddCommand(buildArtifactCmd)
}

// buildArtifactCmdRun validates, builds, and writes a local artifact.
func buildArtifactCmdRun(cmd *cobra.Command, _ []string) (err error) {
	if buildArtifactArgs.output == "" {
		return fmt.Errorf("output path is required")
	}
	format := oci.LocalFormat(buildArtifactArgs.format)
	if err := format.Validate(); err != nil {
		return err
	}
	info, err := os.Stat(buildArtifactArgs.path)
	if err != nil {
		return fmt.Errorf("file path not found %s", buildArtifactArgs.path)
	}
	if buildArtifactArgs.contentType == "" {
		return fmt.Errorf("content type is required")
	}

	var ignorePaths []string
	if info.IsDir() {
		ignorePaths, err = engine.ReadIgnoreFile(buildArtifactArgs.path)
		if err != nil {
			return fmt.Errorf("reading %s failed: %w", apiv1.IgnoreFile, err)
		}
	}
	annotations, err := oci.ParseAnnotations(buildArtifactArgs.annotations)
	if err != nil {
		return err
	}
	oci.AppendGitMetadata(cmd.Context(), buildArtifactArgs.path, annotations)

	// When symlink resolution is enabled, stage the directory in a temp dir
	// so that the artifact contains the symlink targets as regular files
	// and directories, since the archiver skips symbolic links.
	contentPath := buildArtifactArgs.path
	if buildArtifactArgs.resolveSymlinks {
		if info.IsDir() {
			tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
			if err != nil {
				return err
			}
			defer os.RemoveAll(tmpDir)

			contentPath = filepath.Join(tmpDir, "artifact")
			if err := engine.CopyDir(buildArtifactArgs.path, contentPath, true); err != nil {
				return err
			}
		} else {
			contentPath, err = filepath.EvalSymlinks(contentPath)
			if err != nil {
				return err
			}
		}
	}

	build, err := oci.BuildArtifactImage(contentPath, ignorePaths, buildArtifactArgs.contentType, annotations)
	if err != nil {
		return err
	}
	defer func() {
		err = build.CloseWithError(err)
	}()
	if err := oci.WriteImage(build.Image, buildArtifactArgs.output, format, buildArtifactArgs.tags); err != nil {
		return err
	}

	log := LoggerFrom(cmd.Context())
	log.Info(fmt.Sprintf("artifact: %s", logger.ColorizeSubject(filepath.Clean(buildArtifactArgs.output))))
	log.Info(fmt.Sprintf("digest: %s", logger.ColorizeSubject(build.Digest.String())))
	return nil
}
