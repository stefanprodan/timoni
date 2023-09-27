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
	"strings"

	"github.com/fluxcd/pkg/oci/client"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// PushArtifact performs the following operations:
// - packages the content in a tar+gzip layer
// - annotates the layer with the given content type
// - adds the layer to an OpenContainers artifact
// - annotates the artifact with the given annotations
// - uploads the artifact in the container registry
// - returns the digest URL of the upstream artifact
func PushArtifact(ociURL, contentPath string, ignorePaths []string, contentType string, annotations map[string]string, opts []crane.Option) (string, error) {
	if !strings.HasPrefix(ociURL, apiv1.ArtifactPrefix) {
		return "", fmt.Errorf("URL must be in format 'oci://<domain>/<org>/<repo>'")
	}

	ref, err := name.ParseReference(strings.TrimPrefix(ociURL, apiv1.ArtifactPrefix))
	if err != nil {
		return "", fmt.Errorf("parsing '%s' failed: %w", ociURL, err)
	}

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	builder := client.NewClient(nil)

	tgzFile := filepath.Join(tmpDir, "artifact.tgz")
	if err := builder.Build(tgzFile, contentPath, ignorePaths); err != nil {
		return "", fmt.Errorf("packging content failed: %w", err)
	}

	img := mutate.MediaType(empty.Image, types.OCIManifestSchema1)
	img = mutate.ConfigMediaType(img, apiv1.ConfigMediaType)
	img = mutate.Annotations(img, annotations).(gcrv1.Image)

	layer, err := tarball.LayerFromFile(tgzFile, tarball.WithMediaType(apiv1.ContentMediaType))
	if err != nil {
		return "", fmt.Errorf("creating content layer failed: %w", err)
	}

	img, err = mutate.Append(img, mutate.Addendum{
		Layer: layer,
		Annotations: map[string]string{
			apiv1.ContentTypeAnnotation: contentType,
		},
	})
	if err != nil {
		return "", fmt.Errorf("appending content to artifact failed: %w", err)
	}

	if err := crane.Push(img, ref.String(), opts...); err != nil {
		return "", fmt.Errorf("pushing artifact failed: %w", err)
	}

	digest, err := img.Digest()
	if err != nil {
		return "", fmt.Errorf("parsing artifact digest failed: %w", err)
	}

	digestURL := ref.Context().Digest(digest.String()).String()
	return digestURL, nil
}
