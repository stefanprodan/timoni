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
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/flags"
	"github.com/stefanprodan/timoni/internal/logger"
	"github.com/stefanprodan/timoni/internal/oci"
)

var pushModCmd = &cobra.Command{
	Use:   "push MODULE_PATH MODULE_URL",
	Args:  cobra.MaximumNArgs(2),
	Short: "Push a module to a container registry",
	Long: `The push command packages the module as an OCI artifact and pushes it to the
container registry using the version as the image tag.

The push command can also push an OCI archive produced by 'timoni mod build',
extracting the version from the archive manifest.`,
	Example: `  # Push a module to Docker Hub using the credentials from '~/.docker/config.json'
  echo $DOCKER_PAT | docker login --username timoni --password-stdin
  timoni mod push ./path/to/module oci://docker.io/org/app-module -v 1.0.0

  # Push a module to a private registry with explicit credentials
  timoni mod push ./path/to/module oci://registry.example.com/org/app-module \
	--version=1.0.0 \
	--creds user:password

  # Push a pre-release version without marking it as latest
  timoni mod push ./path/to/module oci://docker.io/org/app-module \
	--version=2.0.0-rc.1 \
	--latest=false

  # Push a module that shares files with other modules using symbolic links
  timoni mod push ./path/to/module oci://docker.io/org/app-module \
	--version=1.0.0 \
	--resolve-symlinks

  # Push a module with custom OCI annotations
  timoni mod push ./path/to/module oci://ghcr.io/org/modules/app \
	--version=1.0.0 \
	--annotation='org.opencontainers.image.licenses=Apache-2.0' \
	--annotation='org.opencontainers.image.documentation=https://app.org/docs' \
	--annotation='org.opencontainers.image.description=A timoni.sh module for my app.'

  # Push and sign with Cosign (the cosign binary must be present in PATH)
  echo $GITHUB_TOKEN | timoni registry login ghcr.io -u timoni --password-stdin
  export COSIGN_PASSWORD=password
  timoni mod push ./path/to/module oci://ghcr.io/org/modules/app \
	--version=1.0.0 \
	--sign=cosign \
	--cosign-key=/path/to/cosign.key

  # Push a module and sign it with Cosign Keyless (the cosign binary must be present in PATH)
  echo $GITHUB_TOKEN | timoni registry login ghcr.io -u timoni --password-stdin
  timoni mod push ./path/to/module oci://ghcr.io/org/modules/app \
	--version=1.0.0 \
	--sign=cosign

  # Push a pre-built module OCI archive produced by 'timoni mod build'
  timoni mod push ./module-1.0.0.oci.tar oci://docker.io/org/app-module
`,
	RunE: pushModCmdRun,
}

type pushModFlags struct {
	module          string
	version         flags.Version
	latest          bool
	creds           flags.Credentials
	ignorePaths     []string
	output          string
	annotations     []string
	resolveSymlinks bool
	sign            string
	cosignKey       string
}

var pushModArgs pushModFlags

func init() {
	pushModCmd.Flags().VarP(&pushModArgs.version, pushModArgs.version.Type(), pushModArgs.version.Shorthand(), pushModArgs.version.Description())
	pushModCmd.Flags().Var(&pushModArgs.creds, pushModArgs.creds.Type(), pushModArgs.creds.Description())
	pushModCmd.Flags().BoolVar(&pushModArgs.latest, "latest", true,
		"Tags the current version as the latest stable release.")
	pushModCmd.Flags().StringArrayVarP(&pushModArgs.annotations, "annotation", "a", nil,
		"Set custom OCI annotations in the format '<key>=<value>'.")
	pushModCmd.Flags().StringVarP(&pushModArgs.output, "output", "o", "",
		"The format in which the artifact digest should be printed, can be 'yaml' or 'json'.")
	pushModCmd.Flags().BoolVar(&pushModArgs.resolveSymlinks, "resolve-symlinks", false,
		"Resolve symbolic links and package their targets as regular files and directories.")
	pushModCmd.Flags().StringVar(&pushModArgs.sign, "sign", "",
		"Signs the module with the specified provider.")
	pushModCmd.Flags().StringVar(&pushModArgs.cosignKey, "cosign-key", "",
		"The Cosign private key for signing the module.")

	modCmd.AddCommand(pushModCmd)
}

// pushModCmdRun validates inputs, builds, and pushes a module artifact.
func pushModCmdRun(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("module and URL are required")
	}
	pushModArgs.module = args[0]

	version := pushModArgs.version.String()
	if err := validateOutputFormat(pushModArgs.output, true); err != nil {
		return err
	}
	if err := validateProviderCompanionFlags(cmd, pushModArgs.sign, "sign", "cosign-key"); err != nil {
		return err
	}
	if pushModArgs.sign != "" {
		if err := oci.ValidateSigningProvider(pushModArgs.sign); err != nil {
			return err
		}
	}

	fs, err := os.Stat(pushModArgs.module)
	if err != nil {
		return fmt.Errorf("module not found at path %s", pushModArgs.module)
	}

	// A regular file is a pre-built OCI archive: the version comes from the
	// archive manifest, while a module directory still requires --version.
	if !fs.IsDir() {
		if pushModArgs.resolveSymlinks {
			return fmt.Errorf("--resolve-symlinks is not supported when pushing a pre-built archive")
		}
		if len(pushModArgs.annotations) > 0 {
			return fmt.Errorf("--annotation is not supported when pushing a pre-built archive")
		}
		if version == "" {
			version, err = oci.ModuleVersion(pushModArgs.module)
			if err != nil {
				return err
			}
		}
	} else {
		if err := flags.ValidateModuleVersion(version); err != nil {
			return err
		}
	}

	ociURL := fmt.Sprintf("%s:%s", args[1], version)

	log := LoggerFrom(cmd.Context())

	ctx, cancel := context.WithTimeout(cmd.Context(), rootArgs.timeout)
	defer cancel()

	spin := logger.StartSpinner("pushing module")
	defer spin.Stop()

	opts := oci.Options(ctx, pushModArgs.creds.String(), rootArgs.registryInsecure)

	var digestURL string
	if !fs.IsDir() {
		// Push a pre-built OCI archive as-is.
		digestURL, err = oci.PushModuleArchive(ociURL, pushModArgs.module, version, opts)
		if err != nil {
			return err
		}
	} else {
		annotations, err := oci.ParseAnnotations(pushModArgs.annotations)
		if err != nil {
			return err
		}

		annotations[apiv1.VersionAnnotation] = version
		oci.AppendGitMetadata(cmd.Context(), pushModArgs.module, annotations)

		ps, err := engine.ReadIgnoreFile(pushModArgs.module)
		if err != nil {
			return fmt.Errorf("reading %s failed: %w", apiv1.IgnoreFile, err)
		}
		pushModArgs.ignorePaths = append(pushModArgs.ignorePaths, ps...)

		// When symlink resolution is enabled, stage the module in a temp dir
		// so that the artifact contains the resolved file set local builds
		// see. Without it the archiver skips symlinks by itself, so the
		// module is packaged straight from its source dir.
		moduleDir := pushModArgs.module
		if pushModArgs.resolveSymlinks {
			tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
			if err != nil {
				return err
			}
			defer os.RemoveAll(tmpDir)

			moduleDir = path.Join(tmpDir, "module")
			if err := engine.CopyDir(pushModArgs.module, moduleDir, true); err != nil {
				return err
			}
		}

		digestURL, err = oci.PushModule(ociURL, moduleDir, pushModArgs.ignorePaths, annotations, opts)
		if err != nil {
			return err
		}
	}

	if pushModArgs.latest {
		if err := oci.TagArtifact(digestURL, apiv1.LatestVersion, opts); err != nil {
			return fmt.Errorf("tagging module version as latest failed: %w", err)
		}
	}

	spin.Stop()
	if pushModArgs.sign != "" {
		err = oci.SignArtifact(ctx, log, pushModArgs.sign, digestURL, pushModArgs.cosignKey, rootArgs.registryInsecure, pushModArgs.creds.String())
		if err != nil {
			return err
		}
	}

	digest, err := oci.ParseDigest(digestURL)
	if err != nil {
		return fmt.Errorf("artifact digest parsing failed: %w", err)
	}

	info := struct {
		URL        string `json:"url"`
		Repository string `json:"repository"`
		Version    string `json:"version"`
		Digest     string `json:"digest"`
	}{
		URL:        digestURL,
		Repository: digest.Repository.Name(),
		Version:    version,
		Digest:     digest.DigestStr(),
	}

	switch pushModArgs.output {
	case "json":
		marshalled, err := json.MarshalIndent(&info, "", "  ")
		if err != nil {
			return fmt.Errorf("artifact info JSON conversion failed: %w", err)
		}
		marshalled = append(marshalled, "\n"...)
		cmd.OutOrStdout().Write(marshalled)
	case "yaml":
		marshalled, err := yaml.Marshal(&info)
		if err != nil {
			return fmt.Errorf("artifact info YAML conversion failed: %w", err)
		}
		cmd.OutOrStdout().Write(marshalled)
	default:
		digest, err := oci.ParseDigest(digestURL)
		if err != nil {
			return err
		}
		log.Info(fmt.Sprintf("artifact: %s", logger.ColorizeSubject(ociURL)))
		log.Info(fmt.Sprintf("digest: %s", logger.ColorizeSubject(digest.DigestStr())))
	}

	return nil
}
