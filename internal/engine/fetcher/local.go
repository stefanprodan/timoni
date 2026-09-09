/*
Copyright 2024 Stefan Prodan

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

package fetcher

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/oci"
)

type Local struct {
	src           string
	root          string
	rootErr       error
	requiredFiles []string
	version       string
	digest        string
}

// NewLocal creates a local Fetcher for the given module source directory.
// The source is resolved to an absolute path at construction time, so a
// later working directory change does not alter which module is fetched.
func NewLocal(src string) *Local {
	return newLocal(src, "", "")
}

// NewLocalReference creates a fetcher for a digest-pinned local module source.
// Fetch verifies that the source still matches the expected digest.
func NewLocalReference(src, version, digest string) *Local {
	return newLocal(src, version, digest)
}

func newLocal(src, version, digest string) *Local {
	src = strings.TrimPrefix(src, apiv1.LocalPrefix)
	root, rootErr := filepath.Abs(src)
	if rootErr != nil {
		root = src
	}
	requiredFiles := []string{
		path.Join(root, "cue.mod", "module.cue"),
		path.Join(root, "timoni.cue"),
		path.Join(root, "values.cue"),
	}
	return &Local{
		src:           src,
		root:          root,
		rootErr:       rootErr,
		requiredFiles: requiredFiles,
		version:       version,
		digest:        digest,
	}
}

// LocalArtifact fetches a local OCI module artifact and extracts it into the
// requested destination before the CUE loader reads it.
type LocalArtifact struct {
	src     string
	root    string
	version string
	digest  string
}

// NewLocalArtifact creates a fetcher for a local OCI archive or image layout.
func NewLocalArtifact(src, version, digest, destination string) *LocalArtifact {
	src = strings.TrimPrefix(src, apiv1.LocalPrefix)
	return &LocalArtifact{
		src:     src,
		root:    filepath.Join(destination, "module"),
		version: version,
		digest:  digest,
	}
}

// GetModuleRoot returns the extraction directory of the local OCI artifact.
func (f *LocalArtifact) GetModuleRoot() string { return f.root }

// Fetch verifies and extracts the local OCI artifact.
func (f *LocalArtifact) Fetch() (*apiv1.ModuleReference, error) {
	if f.src == "" {
		return nil, fmt.Errorf("module artifact not found at path %s", f.src)
	}
	if f.root == "module" {
		return nil, fmt.Errorf("destination is required for local OCI artifact %s", f.src)
	}
	if err := os.MkdirAll(f.root, 0o755); err != nil {
		return nil, fmt.Errorf("creating module destination failed: %w", err)
	}
	mod, err := oci.PullLocalModule(f.src, f.root)
	if err != nil {
		return nil, err
	}
	if f.version != "" && f.version != apiv1.LatestVersion && f.version != "@"+mod.Digest && f.version != mod.Version {
		return nil, fmt.Errorf("module artifact %s version mismatch: expected %s, got %s", f.src, f.version, mod.Version)
	}
	if f.digest != "" && f.digest != mod.Digest {
		return nil, fmt.Errorf("module artifact %s digest mismatch: expected %s, got %s", f.src, f.digest, mod.Digest)
	}
	mod.Repository = apiv1.LocalPrefix + f.src
	return mod, nil
}

// GetModuleRoot returns the absolute path of the module source directory.
// Local modules build in place: the CUE loader reads the imported files
// lazily from the source directory and build errors carry the positions
// of the files the user is editing.
func (f *Local) GetModuleRoot() string {
	return f.root
}

// Fetch validates the module source directory and returns the module
// reference. The module is not copied; instance values are injected at
// build time as in-memory overlays, leaving the directory untouched.
func (f *Local) Fetch() (*apiv1.ModuleReference, error) {
	if f.src == "" {
		return nil, fmt.Errorf("module not found at path %s", f.src)
	}

	if f.rootErr != nil {
		return nil, fmt.Errorf("failed to resolve module path %s: %w", f.src, f.rootErr)
	}

	if fs, err := os.Stat(f.root); err != nil || !fs.IsDir() {
		return nil, fmt.Errorf("module not found at path %s", f.src)
	}

	for _, requiredFile := range f.requiredFiles {
		if _, err := os.Stat(requiredFile); err != nil {
			return nil, fmt.Errorf("required file not found: %s", requiredFile)
		}
	}

	version := engine.DefaultDevelVersion
	digest := "unknown"
	if f.digest != "" {
		actual, err := engine.ModuleSourceDigest(f.root)
		if err != nil {
			return nil, fmt.Errorf("failed to verify module source %s: %w", f.src, err)
		}
		if actual != f.digest {
			return nil, fmt.Errorf("module source %s digest mismatch: expected %s, got %s", f.src, f.digest, actual)
		}
		version = f.version
		digest = f.digest
	}

	mr := apiv1.ModuleReference{Repository: f.src, Version: version, Digest: digest}

	return &mr, nil
}
