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
	"github.com/Masterminds/semver/v3"
	oci "github.com/fluxcd/pkg/oci/client"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Fetcher downloads a module and extracts it locally.
type Fetcher struct {
	ctx     context.Context
	src     string
	dst     string
	version string
	creds   string
}

// NewFetcher creates a Fetcher for the given module.
func NewFetcher(ctx context.Context, src, version, dst, creds string) *Fetcher {
	return &Fetcher{
		ctx:     ctx,
		src:     src,
		dst:     dst,
		version: version,
		creds:   creds,
	}
}

func (f *Fetcher) Fetch() (string, error) {
	modulePath := filepath.Join(f.dst, "module")

	if strings.HasPrefix(f.src, "oci://") {
		if err := os.MkdirAll(modulePath, os.ModePerm); err != nil {
			return modulePath, err
		}
		return modulePath, f.fetchOCI(modulePath)
	}

	if fs, err := os.Stat(f.src); err != nil || !fs.IsDir() {
		return modulePath, fmt.Errorf("module not found at path %s", pullArgs.output)
	}
	return modulePath, copyModule(f.src, modulePath)
}

func (f *Fetcher) fetchOCI(dir string) error {
	if _, err := semver.StrictNewVersion(f.version); err != nil {
		return fmt.Errorf("version is not in semver format, error: %w", err)
	}

	ociClient := oci.NewLocalClient()

	if f.creds != "" {
		if err := ociClient.LoginWithCredentials(f.creds); err != nil {
			return fmt.Errorf("could not login with credentials: %w", err)
		}
	}

	url, err := oci.ParseArtifactURL(f.src + ":" + f.version)
	if err != nil {
		return err
	}

	if _, err := ociClient.Pull(f.ctx, url, dir); err != nil {
		return err
	}
	return nil
}

func copyModuleFile(src, dst string) (err error) {
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

	_, err = io.Copy(out, in)
	if err != nil {
		return
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

func copyModule(src string, dst string) (err error) {
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
		return fmt.Errorf("destination already exists")
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
			err = copyModule(srcPath, dstPath)
			if err != nil {
				return
			}
		} else {
			if fi, fiErr := entry.Info(); fiErr != nil || !fi.Mode().IsRegular() {
				return
			}

			err = copyModuleFile(srcPath, dstPath)
			if err != nil {
				return
			}
		}
	}

	return
}
