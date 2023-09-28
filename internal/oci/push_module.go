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
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/compression"
	"github.com/google/go-containerregistry/pkg/crane"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// PushModule performs the following operations:
// - packages the Timoni module's vendored schemas in a dedicated tar+gzip layer
// - packages the Timoni module's templates, values, etc in a 2nd tar+gzip layer
// - adds both layers to an OpenContainers artifact
// - annotates the artifact with the given annotations
// - uploads the module's artifact in the container registry
// - returns the digest URL of the upstream artifact
func PushModule(ociURL, contentPath string, ignorePaths []string, annotations map[string]string, opts []crane.Option) (string, error) {
	ref, err := parseArtifactRef(ociURL)
	if err != nil {
		return "", err
	}

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	img := mutate.MediaType(empty.Image, types.OCIManifestSchema1)
	img = mutate.ConfigMediaType(img, apiv1.ConfigMediaType)
	img = mutate.Annotations(img, annotations).(gcrv1.Image)

	tgzVendor := filepath.Join(tmpDir, "vendor.tgz")
	vendorIgnorePaths := []string{"/*", "!/cue.mod"}
	if err := BuildArtifact(tgzVendor, contentPath, vendorIgnorePaths); err != nil {
		return "", fmt.Errorf("packging vendor layer failed: %w", err)
	}

	layerVendor, err := tarball.LayerFromFile(tgzVendor,
		tarball.WithMediaType(apiv1.ContentMediaType),
		tarball.WithCompression(compression.GZip),
		tarball.WithCompressedCaching,
	)
	if err != nil {
		return "", fmt.Errorf("creating vendor layer failed: %w", err)
	}

	img, err = mutate.Append(img, mutate.Addendum{
		Layer: layerVendor,
		Annotations: map[string]string{
			apiv1.ContentTypeAnnotation: apiv1.CueModContentType,
		},
	})
	if err != nil {
		return "", fmt.Errorf("appending vendor layer to artifact failed: %w", err)
	}

	tgzModule := filepath.Join(tmpDir, "module.tgz")
	ignorePaths = append(ignorePaths, "cue.mod/")
	if err := BuildArtifact(tgzModule, contentPath, ignorePaths); err != nil {
		return "", fmt.Errorf("packging module layer failed: %w", err)
	}

	layerModule, err := tarball.LayerFromFile(tgzModule,
		tarball.WithMediaType(apiv1.ContentMediaType),
		tarball.WithCompression(compression.GZip),
		tarball.WithCompressedCaching,
	)
	if err != nil {
		return "", fmt.Errorf("creating module layer failed: %w", err)
	}

	img, err = mutate.Append(img, mutate.Addendum{
		Layer: layerModule,
		Annotations: map[string]string{
			apiv1.ContentTypeAnnotation: apiv1.TimoniModContentType,
		},
	})
	if err != nil {
		return "", fmt.Errorf("appending module layer to artifact failed: %w", err)
	}

	if err := crane.Push(img, ref.String(), opts...); err != nil {
		return "", fmt.Errorf("pushing artifact failed: %w", err)
	}

	digest, err := img.Digest()
	if err != nil {
		return "", fmt.Errorf("parsing artifact digest failed: %w", err)
	}

	digestURL := ref.Context().Digest(digest.String()).String()
	return fmt.Sprintf("%s%s", apiv1.ArtifactPrefix, digestURL), nil
}
