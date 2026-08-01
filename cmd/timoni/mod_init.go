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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/logger"
	"github.com/stefanprodan/timoni/internal/oci"
)

var initModCmd = &cobra.Command{
	Use:   "init [MODULE NAME] [PATH]",
	Args:  cobra.MaximumNArgs(2),
	Short: "Create a module along with common files and directories",
	Example: `  # Create a module in the current directory
  timoni mod init my-app

  # Create a module at the specified path
  timoni mod init my-app ./modules

  # Create a module from a blueprint
  timoni mod init my-app --blueprint oci://ghcr.io/stefanprodan/timoni/blueprints/starter
`,
	RunE: runInitModCmd,
}

type initModFlags struct {
	name         string
	path         string
	blueprintURL string
}

var initModArgs initModFlags

func init() {
	initModCmd.Flags().StringVarP(&initModArgs.blueprintURL, "blueprint", "b", "", "Blueprint OCI URL")
	modCmd.AddCommand(initModCmd)
}

const (
	modTemplateName = "minimal"
	modTemplateURL  = "oci://ghcr.io/stefanprodan/timoni/minimal"
)

// runInitModCmd initializes a module from an OCI template.
func runInitModCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return errors.New("module name is required")
	}
	initModArgs.name = args[0]

	if len(args) == 2 {
		initModArgs.path = args[1]
	} else {
		initModArgs.path = "."
	}

	log := LoggerFrom(cmd.Context())

	if fs, err := os.Stat(initModArgs.path); err != nil || !fs.IsDir() {
		return fmt.Errorf("path not found: %s", initModArgs.path)
	}

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), rootArgs.timeout)
	defer cancel()

	templateURL := modTemplateURL
	templateName := modTemplateName
	if initModArgs.blueprintURL != "" {
		templateURL = initModArgs.blueprintURL
		templateName = "blueprint"
	}

	spin := logger.StartSpinner(fmt.Sprintf("pulling template from %s", templateURL))
	defer spin.Stop()

	opts := oci.Options(ctx, "", rootArgs.registryInsecure)
	err = oci.PullArtifact(templateURL, tmpDir, apiv1.AnyContentType, opts)
	if err != nil {
		return err
	}

	dst := filepath.Join(initModArgs.path, initModArgs.name)
	err = initializeModule(
		initModArgs.name,
		templateName,
		tmpDir,
		dst,
	)
	if err != nil {
		return err
	}

	spin.Stop()
	log.Info(fmt.Sprintf("module initialized at %s", dst))
	return nil
}

func copyModuleFile(mName, mTmpl, src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		if e := out.Close(); e != nil {
			err = e
		}
	}()

	data, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	txt := strings.Replace(string(data), mTmpl, mName, -1)

	_, err = io.WriteString(out, txt)
	if err != nil {
		return err
	}

	err = out.Sync()
	if err != nil {
		return
	}

	si, err := os.Stat(src)
	if err != nil {
		return
	}

	err = os.Chmod(dst, si.Mode())
	if err != nil {
		return
	}

	return
}

// initializeModule copies a template and writes the default ignore file.
// It removes the newly created destination if either operation fails.
func initializeModule(mName, mTmpl, src, dst string) error {
	if err := initModuleFromTemplate(mName, mTmpl, src, dst); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dst, apiv1.IgnoreFile), []byte(apiv1.DefaultIgnorePatterns), 0600); err != nil {
		_ = os.RemoveAll(dst)
		return err
	}
	return nil
}

// initModuleFromTemplate copies a template into a new module destination.
// It removes the destination if initialization fails.
func initModuleFromTemplate(mName, mTmpl, src string, dst string) (err error) {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !si.IsDir() {
		return errors.New("source is not a directory")
	}

	if err = os.MkdirAll(filepath.Dir(dst), si.Mode()); err != nil {
		return
	}
	err = os.Mkdir(dst, si.Mode())
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("module %s already exists", dst)
	}
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(dst)
		}
	}()

	entries, err := os.ReadDir(src)
	if err != nil {
		return
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			err = initModuleFromTemplate(mName, mTmpl, srcPath, dstPath)
			if err != nil {
				return
			}
		} else {
			fi, fiErr := entry.Info()
			if fiErr != nil {
				return fmt.Errorf("reading template entry %s failed: %w", srcPath, fiErr)
			}
			if !fi.Mode().IsRegular() {
				return fmt.Errorf("template entry is not a regular file: %s", srcPath)
			}

			err = copyModuleFile(mName, mTmpl, srcPath, dstPath)
			if err != nil {
				return
			}
		}
	}

	return err
}
