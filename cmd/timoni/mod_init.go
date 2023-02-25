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

	oci "github.com/fluxcd/pkg/oci/client"
	"github.com/spf13/cobra"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

var initModCmd = &cobra.Command{
	Use:   "init [MODULE NAME] [PATH]",
	Short: "Create a module along with common files and directories",
	Example: `  # create a module in the current directory
  timoni mod init my-app .
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
	modTemplateName      = "podinfo"
	modTemplateURL       = "ghcr.io/stefanprodan/modules/podinfo"
	modTemplateImageRepo = "ghcr.io/stefanprodan"
)

func runInitModCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("module name and path are required")
	}

	initModArgs.name = args[0]
	initModArgs.path = args[1]

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

	ociClient := oci.NewClient(nil)

	if _, err := ociClient.Pull(ctx, modTemplateURL, tmpDir); err != nil {
		return err
	}

	return initModuleFromTemplate(
		initModArgs.name,
		modTemplateName,
		tmpDir,
		filepath.Join(initModArgs.path, initModArgs.name),
	)
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

	if filepath.Base(in.Name()) == "README.md" {
		_, err = io.Copy(out, in)
		if err != nil {
			return
		}
	} else {
		data, err := io.ReadAll(in)
		if err != nil {
			return err
		}
		txt := strings.Replace(string(data), mTmpl, mName, -1)

		// TODO: find a better way to preserve the container image original name
		txt = strings.Replace(
			txt,
			fmt.Sprintf("%s/%s", modTemplateImageRepo, mName),
			fmt.Sprintf("%s/%s", modTemplateImageRepo, mTmpl),
			-1,
		)
		_, err = io.WriteString(out, txt)
		if err != nil {
			return err
		}
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

	return
}
