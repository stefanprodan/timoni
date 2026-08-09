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

package oci

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fluxcd/pkg/tar"
	"github.com/google/go-containerregistry/pkg/crane"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// PushModule builds and pushes ordered vendor and module layers, then returns
// the module's digest URL.
func PushModule(ociURL, contentPath string, ignorePaths []string, annotations map[string]string, opts []crane.Option) (result string, err error) {
	ref, err := parseArtifactRef(ociURL)
	if err != nil {
		return "", err
	}

	build, err := BuildModuleImage(contentPath, ignorePaths, annotations)
	if err != nil {
		return "", err
	}
	defer func() {
		err = build.CloseWithError(err)
	}()

	if err := crane.Push(build.Image, ref.String(), opts...); err != nil {
		return "", fmt.Errorf("pushing artifact failed: %w", err)
	}

	digestURL := ref.Context().Digest(build.Digest.String()).String()
	return fmt.Sprintf("%s%s", apiv1.ArtifactPrefix, digestURL), nil
}

// PushModuleArchive pushes a pre-built module image from a local OCI archive
// or OCI layout to a registry, then returns the module's digest URL. The
// version must match the version the image was built with.
func PushModuleArchive(ociURL, archivePath, version string, opts []crane.Option) (result string, err error) {
	ref, err := parseArtifactRef(ociURL)
	if err != nil {
		return "", err
	}

	image, digest, cleanup, err := loadLocalImage(archivePath, version)
	if err != nil {
		return "", err
	}
	defer func() {
		err = errors.Join(err, cleanup())
	}()

	if err := crane.Push(image, ref.String(), opts...); err != nil {
		return "", fmt.Errorf("pushing artifact failed: %w", err)
	}

	digestURL := ref.Context().Digest(digest.String()).String()
	return fmt.Sprintf("%s%s", apiv1.ArtifactPrefix, digestURL), nil
}

// IsOCILayout reports whether path is an OCI image layout directory, that is
// a directory written by `mod build --format=oci-layout` rather than a module
// source directory. It is cheap to call and reads no blobs.
func IsOCILayout(path string) bool {
	for _, name := range []string{"oci-layout", "index.json"} {
		info, err := os.Stat(filepath.Join(path, name))
		if err != nil || info.IsDir() {
			return false
		}
	}
	return true
}

// loadLocalImage loads a Timoni module image from a local OCI archive or OCI
// layout. The version matches the local reference stored in the layout index;
// when no descriptor carries it, the first image is returned. The returned
// image reads its blobs lazily from disk, so the caller must run the returned
// cleanup function after the image is consumed.
func loadLocalImage(source, version string) (gcrv1.Image, gcrv1.Hash, func() error, error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, gcrv1.Hash{}, nil, fmt.Errorf("module not found at path %s", source)
	}

	image, cleanup, err := imageFromLocal(source, info.IsDir(), version)
	if err != nil {
		if cleanup != nil {
			_ = cleanup()
		}
		return nil, gcrv1.Hash{}, nil, err
	}

	manifest, err := image.Manifest()
	if err != nil {
		_ = cleanup()
		return nil, gcrv1.Hash{}, nil, fmt.Errorf("reading artifact manifest failed: %w", err)
	}
	if manifest.Config.MediaType != apiv1.ConfigMediaType {
		_ = cleanup()
		return nil, gcrv1.Hash{}, nil, fmt.Errorf("unsupported artifact type '%s', must be '%s'",
			manifest.Config.MediaType, apiv1.ConfigMediaType)
	}
	if builtVersion, ok := manifest.Annotations[apiv1.VersionAnnotation]; ok && builtVersion != version {
		_ = cleanup()
		return nil, gcrv1.Hash{}, nil, fmt.Errorf("version mismatch: archive was built with version %s, cannot push as %s",
			builtVersion, version)
	}

	digest, err := image.Digest()
	if err != nil {
		_ = cleanup()
		return nil, gcrv1.Hash{}, nil, fmt.Errorf("calculating artifact digest failed: %w", err)
	}
	return image, digest, cleanup, nil
}

// imageFromLocal loads the first image of a local OCI layout directory or,
// when source is a file, extracts the archive to a temporary directory that
// stays alive until the returned cleanup function runs.
func imageFromLocal(source string, isDir bool, version string) (gcrv1.Image, func() error, error) {
	path := source
	cleanup := func() error { return nil }
	if !isDir {
		tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
		if err != nil {
			return nil, nil, err
		}
		cleanup = func() error { return os.RemoveAll(tmpDir) }

		archive, err := os.Open(source)
		if err != nil {
			return nil, cleanup, fmt.Errorf("opening OCI archive failed: %w", err)
		}
		defer archive.Close()

		// Archives are plain tar streams of an OCI image layout.
		if err := tar.Untar(archive, tmpDir,
			tar.WithSkipGzip(),
			tar.WithSkipSymlinks(),
			tar.WithMaxUntarSize(tar.UnlimitedUntarSize),
		); err != nil {
			return nil, cleanup, fmt.Errorf("extracting OCI archive failed: %w", err)
		}
		path = tmpDir
	}

	layoutPath, err := layout.FromPath(path)
	if err != nil {
		return nil, cleanup, fmt.Errorf("opening OCI layout failed: %w", err)
	}
	index, err := layoutPath.ImageIndex()
	if err != nil {
		return nil, cleanup, fmt.Errorf("reading OCI layout failed: %w", err)
	}
	manifest, err := index.IndexManifest()
	if err != nil {
		return nil, cleanup, fmt.Errorf("reading OCI layout failed: %w", err)
	}
	if len(manifest.Manifests) == 0 {
		return nil, cleanup, fmt.Errorf("no image found in OCI layout")
	}

	for _, desc := range manifest.Manifests {
		if desc.Annotations["org.opencontainers.image.ref.name"] == version {
			image, err := index.Image(desc.Digest)
			return image, cleanup, err
		}
	}

	image, err := index.Image(manifest.Manifests[0].Digest)
	return image, cleanup, err
}
