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

	"github.com/fluxcd/pkg/tar"
	"github.com/google/go-containerregistry/pkg/crane"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/types"

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
// to a registry, then returns the module's digest URL. The version must match
// the version annotation baked into the archive.
func PushModuleArchive(ociURL, archivePath, version string, opts []crane.Option) (result string, err error) {
	ref, err := parseArtifactRef(ociURL)
	if err != nil {
		return "", err
	}

	image, cleanup, err := imageFromLocal(archivePath)
	if err != nil {
		return "", err
	}
	defer func() {
		err = errors.Join(err, cleanup())
	}()

	manifest, err := image.Manifest()
	if err != nil {
		return "", fmt.Errorf("reading artifact manifest failed: %w", err)
	}
	if err := validateModuleManifest(manifest, version); err != nil {
		return "", err
	}

	if err := crane.Push(image, ref.String(), opts...); err != nil {
		return "", fmt.Errorf("pushing artifact failed: %w", err)
	}

	digest, err := image.Digest()
	if err != nil {
		return "", fmt.Errorf("calculating artifact digest failed: %w", err)
	}
	digestURL := ref.Context().Digest(digest.String()).String()
	return fmt.Sprintf("%s%s", apiv1.ArtifactPrefix, digestURL), nil
}

// ModuleVersion reads the module version annotation from a local OCI archive.
func ModuleVersion(archivePath string) (string, error) {
	image, cleanup, err := imageFromLocal(archivePath)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = cleanup()
	}()

	manifest, err := image.Manifest()
	if err != nil {
		return "", fmt.Errorf("reading artifact manifest failed: %w", err)
	}
	version, ok := manifest.Annotations[apiv1.VersionAnnotation]
	if !ok {
		return "", fmt.Errorf("module version annotation is missing")
	}
	return version, nil
}

// validateModuleManifest verifies that the manifest describes a Timoni module
// artifact: an OCI manifest with the Timoni config media type, the expected
// version annotation, and exactly two ordered vendor and module layers.
func validateModuleManifest(manifest *gcrv1.Manifest, version string) error {
	if manifest.MediaType != types.OCIManifestSchema1 {
		return fmt.Errorf("unsupported manifest media type %q", manifest.MediaType)
	}

	if manifest.Config.MediaType != apiv1.ConfigMediaType {
		return fmt.Errorf("unsupported config media type %q", manifest.Config.MediaType)
	}

	if got, ok := manifest.Annotations[apiv1.VersionAnnotation]; !ok {
		return fmt.Errorf("module version annotation is missing")
	} else if got != version {
		return fmt.Errorf(
			"version mismatch: archive was built with version %s, cannot push as %s",
			got, version,
		)
	}

	if len(manifest.Layers) != 2 {
		return fmt.Errorf("invalid module artifact: expected 2 layers, got %d", len(manifest.Layers))
	}

	expected := []string{
		apiv1.TimoniModVendorContentType,
		apiv1.TimoniModContentType,
	}

	for i, layer := range manifest.Layers {
		if layer.MediaType != apiv1.ContentMediaType {
			return fmt.Errorf("invalid module layer %d media type %q", i, layer.MediaType)
		}

		if got := layer.Annotations[apiv1.ContentTypeAnnotation]; got != expected[i] {
			return fmt.Errorf(
				"invalid module layer %d content type %q, expected %q",
				i, got, expected[i],
			)
		}
	}

	return nil
}

// imageFromLocal loads the first image of an OCI archive, extracting the
// plain tar stream to a temporary directory that stays alive until the
// returned cleanup function runs.
func imageFromLocal(source string) (gcrv1.Image, func() error, error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, nil, fmt.Errorf("module not found at path %s", source)
	}
	if info.IsDir() {
		return nil, nil, fmt.Errorf("module not found at path %s", source)
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
	defer func() {
		_ = archive.Close()
	}()

	// Archives are plain tar streams of an OCI image layout.
	if err := tar.Untar(archive, tmpDir,
		tar.WithSkipGzip(),
		tar.WithSkipSymlinks(),
	); err != nil {
		return nil, cleanup, fmt.Errorf("extracting OCI archive failed: %w", err)
	}

	layoutPath, err := layout.FromPath(tmpDir)
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

	image, err := index.Image(manifest.Manifests[0].Digest)
	return image, cleanup, err
}
