/*
Copyright 2026 Stefan Prodan

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
	"fmt"
	"io"
	"os"

	"github.com/fluxcd/pkg/tar"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// LocalModuleMetadata returns the semantic version and manifest digest of a
// local Timoni module archive or OCI image layout.
func LocalModuleMetadata(source string) (string, string, error) {
	_, cleanup, _, version, digest, err := localModule(source)
	if err != nil {
		return "", "", err
	}
	if err := cleanup(); err != nil {
		return "", "", err
	}
	return version, digest, nil
}

// PullLocalModule extracts a local Timoni module archive or OCI image layout
// into dstPath and returns its manifest metadata.
func PullLocalModule(source, dstPath string) (*apiv1.ModuleReference, error) {
	image, cleanup, manifest, version, digest, err := localModule(source)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cleanup() }()

	layers, err := image.Layers()
	if err != nil {
		return nil, fmt.Errorf("reading module layers failed: %w", err)
	}
	for i, descriptor := range manifest.Layers {
		if descriptor.MediaType != apiv1.ContentMediaType {
			continue
		}
		reader, err := layers[i].Uncompressed()
		if err != nil {
			return nil, fmt.Errorf("reading module layer %d failed: %w", i, err)
		}
		extractErr := extractUncompressedLayer(reader, dstPath)
		closeErr := reader.Close()
		if extractErr != nil {
			return nil, fmt.Errorf("extracting module layer %d failed: %w", i, extractErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("closing module layer %d failed: %w", i, closeErr)
		}
	}

	return &apiv1.ModuleReference{
		Version:     version,
		Digest:      digest,
		Annotations: manifest.Annotations,
	}, nil
}

func localModule(source string) (gcrv1.Image, func() error, *gcrv1.Manifest, string, string, error) {
	image, cleanup, err := imageFromLocalArtifact(source)
	if err != nil {
		return nil, nil, nil, "", "", err
	}
	fail := func(err error) (gcrv1.Image, func() error, *gcrv1.Manifest, string, string, error) {
		_ = cleanup()
		return nil, nil, nil, "", "", err
	}
	manifest, err := image.Manifest()
	if err != nil {
		return fail(fmt.Errorf("reading artifact manifest failed: %w", err))
	}
	version := manifest.Annotations[apiv1.VersionAnnotation]
	if err := validateModuleManifest(manifest, version); err != nil {
		return fail(err)
	}
	digest, err := image.Digest()
	if err != nil {
		return fail(fmt.Errorf("calculating artifact digest failed: %w", err))
	}
	return image, cleanup, manifest, version, digest.String(), nil
}

// imageFromLocalArtifact loads the first image from an OCI archive or layout.
func imageFromLocalArtifact(source string) (gcrv1.Image, func() error, error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, nil, fmt.Errorf("module artifact not found at path %s", source)
	}
	if info.IsDir() {
		return imageFromLayoutPath(source)
	}

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() error { return os.RemoveAll(tmpDir) }
	archive, err := os.Open(source)
	if err != nil {
		return nil, cleanup, fmt.Errorf("opening OCI archive failed: %w", err)
	}
	defer archive.Close()
	if err := untarLocalArchive(archive, tmpDir); err != nil {
		_ = cleanup()
		return nil, nil, err
	}
	image, _, err := imageFromLayoutPath(tmpDir)
	if err != nil {
		_ = cleanup()
		return nil, nil, err
	}
	return image, cleanup, nil
}

func untarLocalArchive(r io.Reader, dst string) error {
	if err := tar.Untar(r, dst, tar.WithSkipGzip(), tar.WithSkipSymlinks()); err != nil {
		return fmt.Errorf("extracting OCI archive failed: %w", err)
	}
	return nil
}

func imageFromLayoutPath(source string) (gcrv1.Image, func() error, error) {
	layoutPath, err := layout.FromPath(source)
	if err != nil {
		return nil, nil, fmt.Errorf("opening OCI layout failed: %w", err)
	}
	index, err := layoutPath.ImageIndex()
	if err != nil {
		return nil, nil, fmt.Errorf("reading OCI layout failed: %w", err)
	}
	manifest, err := index.IndexManifest()
	if err != nil {
		return nil, nil, fmt.Errorf("reading OCI layout failed: %w", err)
	}
	if len(manifest.Manifests) == 0 {
		return nil, nil, fmt.Errorf("no image found in OCI layout")
	}
	image, err := index.Image(manifest.Manifests[0].Digest)
	if err != nil {
		return nil, nil, err
	}
	return image, func() error { return nil }, nil
}
