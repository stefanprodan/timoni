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
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/compression"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// ImageBuild owns a completed OCI image, its digest, and temporary layer files.
type ImageBuild struct {
	Image  gcrv1.Image
	Digest gcrv1.Hash
	tmpDir string
}

// Close removes the temporary layer files owned by the build.
func (b *ImageBuild) Close() error {
	return os.RemoveAll(b.tmpDir)
}

// contentLayer defines one ordered layer in an OCI image build.
type contentLayer struct {
	name        string
	ignorePaths []string
	contentType string
}

// buildImage packages the layers in order and computes the completed image digest.
func buildImage(contentPath string, layers []contentLayer, annotations map[string]string) (_ *ImageBuild, err error) {
	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(tmpDir)
		}
	}()

	img := mutate.MediaType(empty.Image, types.OCIManifestSchema1)
	img = mutate.ConfigMediaType(img, apiv1.ConfigMediaType)
	img = mutate.Annotations(img, annotations).(gcrv1.Image)

	for _, spec := range layers {
		tgzFile := filepath.Join(tmpDir, spec.name+".tgz")
		if err := BuildArtifact(tgzFile, contentPath, spec.ignorePaths); err != nil {
			return nil, fmt.Errorf("packaging %s layer failed: %w", spec.name, err)
		}

		layer, err := tarball.LayerFromFile(tgzFile,
			tarball.WithMediaType(apiv1.ContentMediaType),
			tarball.WithCompression(compression.GZip),
			tarball.WithCompressedCaching,
		)
		if err != nil {
			return nil, fmt.Errorf("creating %s layer failed: %w", spec.name, err)
		}

		img, err = mutate.Append(img, mutate.Addendum{
			Layer: layer,
			Annotations: map[string]string{
				apiv1.ContentTypeAnnotation: spec.contentType,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("appending %s layer failed: %w", spec.name, err)
		}
	}

	digest, err := img.Digest()
	if err != nil {
		return nil, fmt.Errorf("calculating artifact digest failed: %w", err)
	}

	return &ImageBuild{Image: img, Digest: digest, tmpDir: tmpDir}, nil
}

// BuildArtifactImage packages contentPath as a single-layer OCI image.
func BuildArtifactImage(contentPath string, ignorePaths []string, contentType string, annotations map[string]string) (*ImageBuild, error) {
	return buildImage(contentPath, []contentLayer{{
		name:        "artifact",
		ignorePaths: ignorePaths,
		contentType: contentType,
	}}, annotations)
}

// BuildModuleImage packages a module as ordered vendor and module layers.
func BuildModuleImage(contentPath string, ignorePaths []string, annotations map[string]string) (*ImageBuild, error) {
	// Copy caller rules before excluding vendored schemas from the module layer.
	moduleIgnore := append(append([]string{}, ignorePaths...), "cue.mod/")
	return buildImage(contentPath, []contentLayer{
		{name: "vendor", ignorePaths: []string{"/*", "!/cue.mod"}, contentType: apiv1.TimoniModVendorContentType},
		{name: "module", ignorePaths: moduleIgnore, contentType: apiv1.TimoniModContentType},
	}, annotations)
}
