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

	"github.com/fluxcd/pkg/tar"
	"github.com/google/go-containerregistry/pkg/crane"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// PullArtifact performs the following operations:
// - fetches the manifest of the remote artifact
// - verifies that artifact config matches Timoni's media type
// - download all the compressed layer matching Timoni's media type
// - extracts the layers contents to the destination directory
func PullArtifact(ociURL, dstPath, contentType string, opts []crane.Option) error {
	ref, err := parseArtifactRef(ociURL)
	if err != nil {
		return err
	}

	repoURL := ref.Context().Name()

	manifestJSON, err := crane.Manifest(ref.String(), opts...)
	if err != nil {
		return fmt.Errorf("pulling artifact manifest failed: %w", err)
	}

	manifest, err := gcrv1.ParseManifest(bytes.NewReader(manifestJSON))
	if err != nil {
		return fmt.Errorf("parsing artifact manifest failed: %w", err)
	}

	if manifest.Config.MediaType != apiv1.ConfigMediaType {
		return fmt.Errorf("unsupported artifact type '%s', must be '%s'",
			manifest.Config.MediaType, apiv1.ConfigMediaType)
	}

	var found bool
	for _, layer := range manifest.Layers {
		if layer.MediaType == apiv1.ContentMediaType {
			if contentType != apiv1.AnyContentType && layer.Annotations[apiv1.ContentTypeAnnotation] != contentType {
				continue
			}
			found = true
			layerDigest := layer.Digest.String()
			blobURL := fmt.Sprintf("%s@%s", repoURL, layerDigest)
			layer, err := crane.PullLayer(blobURL, opts...)
			if err != nil {
				return fmt.Errorf("pulling artifact layer %s failed: %w", layerDigest, err)
			}

			blob, err := layer.Compressed()
			if err != nil {
				return fmt.Errorf("extracting artifact layer %s failed: %w", layerDigest, err)
			}

			if err = tar.Untar(blob, dstPath, tar.WithMaxUntarSize(-1)); err != nil {
				return fmt.Errorf("extracting artifact layer %s failed: %w", layerDigest, err)
			}
		}
	}

	if !found {
		if contentType != "" {
			return fmt.Errorf(
				"no layer found in artifact with media type '%s' and content type '%s'",
				apiv1.ContentMediaType,
				contentType,
			)
		}
		return fmt.Errorf("no layer found in artifact with media type '%s'", apiv1.ContentMediaType)
	}

	return nil
}
