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
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	oci "github.com/fluxcd/pkg/oci/client"
	"github.com/google/go-containerregistry/pkg/crane"
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
	ociURL := fmt.Sprintf("%s:%s", f.src, f.version)

	if strings.HasPrefix(f.version, "@") {
		ociURL = fmt.Sprintf("%s%s", f.src, f.version)
	} else {
		if _, err := semver.StrictNewVersion(f.version); f.version != LatestTag && err != nil {
			return nil, fmt.Errorf("version is not in semver format: %w", err)
		}
	}

	ociClient := oci.NewClient(nil)

	if f.creds != "" {
		if err := ociClient.LoginWithCredentials(f.creds); err != nil {
			return nil, fmt.Errorf("could not login with credentials: %w", err)
		}
	}

	url, err := oci.ParseArtifactURL(ociURL)
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

type ModuleVersion struct {
	Number string
	Digest string
}

// GetVersions returns a list of OCI tags and their digests.
// The list is ordered based on semver, newest version first.
func (f *Fetcher) GetVersions() ([]ModuleVersion, error) {
	var result []ModuleVersion

	ociClient := oci.NewClient(nil)

	if f.creds != "" {
		if err := ociClient.LoginWithCredentials(f.creds); err != nil {
			return nil, fmt.Errorf("could not login with credentials: %w", err)
		}
	}

	url, err := oci.ParseRepositoryURL(f.src)
	if err != nil {
		return nil, err
	}

	opts := ociClient.GetOptions()
	opts = append(opts, crane.WithContext(f.ctx))

	tags, err := crane.ListTags(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("listing tags failed: %w", err)
	}

	var versions []*semver.Version
	for _, tag := range tags {
		if v, err := semver.StrictNewVersion(tag); err != nil {
			continue
		} else {
			versions = append(versions, v)
		}
	}
	sort.Sort(sort.Reverse(semver.Collection(versions)))

	if digest, err := crane.Digest(fmt.Sprintf("%s:%s", url, LatestTag), opts...); err == nil {
		result = append(result, ModuleVersion{
			Number: LatestTag,
			Digest: digest,
		})
	}

	// TODO: parallelize the digest calls in batches of go routines to speed up
	for _, v := range versions {
		digest, err := crane.Digest(fmt.Sprintf("%s:%s", url, v.String()), opts...)
		if err != nil {
			return nil, fmt.Errorf("faild to get digest for '%s': %w", v.String(), err)
		}
		result = append(result, ModuleVersion{
			Number: v.String(),
			Digest: digest,
		})
	}

	return result, nil
}
