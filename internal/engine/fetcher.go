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
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"
	oci "github.com/fluxcd/pkg/oci/client"
	gcr "github.com/google/go-containerregistry/pkg/name"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// LatestTag is the OCI tag name used to denote the latest stable version of a module.
const LatestTag = "latest"

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

func (f *Fetcher) GetModuleRoot() string {
	return filepath.Join(f.dst, "module")
}

// Fetch downloads a remote module locally into tmp.
func (f *Fetcher) Fetch() (*apiv1.ModuleReference, error) {
	modulePath := f.GetModuleRoot()

	if strings.HasPrefix(f.src, "oci://") {
		if err := os.MkdirAll(modulePath, os.ModePerm); err != nil {
			return nil, err
		}
		return f.fetchOCI(modulePath)
	}

	if fs, err := os.Stat(f.src); err != nil || !fs.IsDir() {
		return nil, fmt.Errorf("module not found at path %s", f.src)
	}

	mr := apiv1.ModuleReference{
		Repository: f.src,
		Version:    "devel",
		Digest:     "unknown",
	}

	return &mr, CopyModule(f.src, modulePath)
}

func (f *Fetcher) fetchOCI(dir string) (*apiv1.ModuleReference, error) {
	if _, err := semver.StrictNewVersion(f.version); f.version != LatestTag && err != nil {
		return nil, fmt.Errorf("version is not in semver format, error: %w", err)
	}

	ociClient := oci.NewClient(nil)

	if f.creds != "" {
		if err := ociClient.LoginWithCredentials(f.creds); err != nil {
			return nil, fmt.Errorf("could not login with credentials: %w", err)
		}
	}

	url, err := oci.ParseArtifactURL(f.src + ":" + f.version)
	if err != nil {
		return nil, err
	}

	meta, err := ociClient.Pull(f.ctx, url, dir)
	if err != nil {
		return nil, err
	}

	digest, err := gcr.NewDigest(meta.Digest)
	if err != nil {
		return nil, err
	}

	mr := apiv1.ModuleReference{
		Repository: f.src,
		Version:    meta.Revision,
		Digest:     digest.DigestStr(),
	}

	return &mr, nil
}
