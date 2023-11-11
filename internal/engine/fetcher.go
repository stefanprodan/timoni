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

package engine

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/oci"
)

// Fetcher downloads a module and extracts it locally.
type Fetcher struct {
	ctx      context.Context
	src      string
	dst      string
	cacheDir string
	version  string
	creds    string
}

// NewFetcher creates a Fetcher for the given module.
func NewFetcher(ctx context.Context, src, version, dst, cacheDir, creds string) *Fetcher {
	return &Fetcher{
		ctx:      ctx,
		src:      src,
		dst:      dst,
		version:  version,
		cacheDir: cacheDir,
		creds:    creds,
	}
}

func (f *Fetcher) GetModuleRoot() string {
	return filepath.Join(f.dst, "module")
}

// Fetch copies the module contents to the destination directory.
// If the module source is a remote OCI repository, the artifact is pulled
// from the registry and its contents extracted to the destination dir.
// If the module source is a local directory, the module required
// files are validated and the module contents is copied to the
// destination dir while excluding files based on the timoni.ignore patters.
func (f *Fetcher) Fetch() (*apiv1.ModuleReference, error) {
	dstDir := f.GetModuleRoot()

	if strings.HasPrefix(f.src, "oci://") {
		return f.fetchRemoteModule(dstDir)
	}

	return f.fetchLocalModule(dstDir)
}

func (f *Fetcher) fetchLocalModule(dstDir string) (*apiv1.ModuleReference, error) {
	if fs, err := os.Stat(f.src); err != nil || !fs.IsDir() {
		return nil, fmt.Errorf("module not found at path %s", f.src)
	}

	modFile := path.Join(f.src, "cue.mod", "module.cue")
	timoniFile := path.Join(f.src, "timoni.cue")
	valuesFile := path.Join(f.src, "values.cue")

	for _, requiredFile := range []string{modFile, timoniFile, valuesFile} {
		if _, err := os.Stat(requiredFile); err != nil {
			return nil, fmt.Errorf("required file not found: %s", requiredFile)
		}
	}

	mr := apiv1.ModuleReference{
		Repository: f.src,
		Version:    defaultDevelVersion,
		Digest:     "unknown",
	}

	return &mr, CopyModule(f.src, dstDir)
}

func (f *Fetcher) fetchRemoteModule(dstDir string) (*apiv1.ModuleReference, error) {
	ociURL := fmt.Sprintf("%s:%s", f.src, f.version)
	if strings.HasPrefix(f.version, "@") {
		ociURL = fmt.Sprintf("%s%s", f.src, f.version)
	}

	if err := os.MkdirAll(dstDir, os.ModePerm); err != nil {
		return nil, err
	}

	opts := oci.Options(f.ctx, f.creds)
	return oci.PullModule(ociURL, dstDir, f.cacheDir, opts)
}
