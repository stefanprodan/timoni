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
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/oci"
)

var initModCmd = &cobra.Command{
	Use:   "init [MODULE NAME] [PATH]",
	Short: "Create a module along with common files and directories",
	Example: `  # Create a module in the current directory
  timoni mod init my-app

  # Create a module at the specified path
  timoni mod init my-app ./modules
`,
	RunE: runInitModCmd,
}

type initModFlags struct {
	name string
	path string
}

var initModArgs initModFlags

func init() {
	modCmd.AddCommand(initModCmd)
}

const (
	modTemplateName = "minimal"
	modTemplateURL  = "oci://ghcr.io/stefanprodan/timoni/minimal"
)

func runInitModCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("module name is required")
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

	spin := StartSpinner(fmt.Sprintf("pulling template from %s", modTemplateURL))
	defer spin.Stop()

	opts := oci.Options(ctx, "")
	err = oci.PullArtifact(modTemplateURL, tmpDir, apiv1.AnyContentType, opts)
	if err != nil {
		return err
	}

	dst := filepath.Join(initModArgs.path, initModArgs.name)
	err = initModuleFromTemplate(
		initModArgs.name,
		modTemplateName,
		tmpDir,
		dst,
	)
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(dst, apiv1.IgnoreFile), []byte(apiv1.DefaultIgnorePatterns), 0600)
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

func initModuleFromTemplate(mName, mTmpl, src string, dst string) (err error) {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !si.IsDir() {
		return fmt.Errorf("source is not a directory")
	}

	_, err = os.Stat(dst)
	if err != nil && !os.IsNotExist(err) {
		return
	}
	if err == nil {
		return fmt.Errorf("module %s already exists", dst)
	}

	err = os.MkdirAll(dst, si.Mode())
	if err != nil {
		return
	}

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
			if fi, fiErr := entry.Info(); fiErr != nil || !fi.Mode().IsRegular() {
				return
			}

			err = copyModuleFile(mName, mTmpl, srcPath, dstPath)
			if err != nil {
				return
			}
		}
	}

	return err
}
