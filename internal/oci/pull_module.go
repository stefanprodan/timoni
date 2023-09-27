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
	"bytes"
	"fmt"
	"strings"

	"github.com/fluxcd/pkg/tar"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// PullModule performs the following operations:
// - determines the artifact digest corresponding to the module version
// - fetches the manifest of the remote artifact
// - verifies that artifact config matches Timoni's media type
// - download all the compressed layer matching Timoni's media type
// - extracts the module contents to the destination directory
func PullModule(ociURL, dstPath string, opts []crane.Option) (*apiv1.ModuleReference, error) {
	if !strings.HasPrefix(ociURL, apiv1.ArtifactPrefix) {
		return nil, fmt.Errorf("URL must be in format 'oci://<domain>/<org>/<repo>'")
	}

	imgURL := strings.TrimPrefix(ociURL, apiv1.ArtifactPrefix)
	ref, err := name.ParseReference(imgURL)
	if err != nil {
		return nil, fmt.Errorf("'%s' invalid URL: %w", ociURL, err)
	}

	repoURL := ref.Context().Name()

	digest, err := crane.Digest(imgURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("resolving the digest for '%s' failed: %w", ociURL, err)
	}

	manifestJSON, err := crane.Manifest(imgURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("pulling artifact manifest failed: %w", err)
	}

	manifest, err := gcrv1.ParseManifest(bytes.NewReader(manifestJSON))
	if err != nil {
		return nil, fmt.Errorf("parsing artifact manifest failed: %w", err)
	}

	if manifest.Config.MediaType != apiv1.ConfigMediaType {
		return nil, fmt.Errorf("unsupported artifact type '%s', must be '%s'",
			manifest.Config.MediaType, apiv1.ConfigMediaType)
	}

	moduleRef := &apiv1.ModuleReference{
		Repository: fmt.Sprintf("%s%s", apiv1.ArtifactPrefix, repoURL),
		Version:    manifest.Annotations[apiv1.RevisionAnnotation],
		Digest:     digest,
	}

	var foundLayer bool
	for _, layer := range manifest.Layers {
		if layer.MediaType == apiv1.ContentMediaType {
			foundLayer = true
			layerDigest := layer.Digest.String()
			blobURL := fmt.Sprintf("%s@%s", repoURL, layerDigest)
			layer, err := crane.PullLayer(blobURL, opts...)
			if err != nil {
				return nil, fmt.Errorf("pulling layer %s failed: %w", layerDigest, err)
			}

			blob, err := layer.Compressed()
			if err != nil {
				return nil, fmt.Errorf("extracting layer %s failed: %w", layerDigest, err)
			}

			if err = tar.Untar(blob, dstPath, tar.WithMaxUntarSize(-1)); err != nil {
				return nil, fmt.Errorf("extracting layer %s failed: %w", layerDigest, err)
			}
		}
	}

	if !foundLayer {
		return nil, fmt.Errorf("no layer found in artifact with media type '%s'", apiv1.ContentMediaType)
	}

	return moduleRef, nil
}
