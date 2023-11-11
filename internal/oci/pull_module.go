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
	"io"
	"os"
	"path"

	"github.com/fluxcd/pkg/tar"
	"github.com/google/go-containerregistry/pkg/crane"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// PullModule performs the following operations:
// - determines the artifact digest corresponding to the module version
// - fetches the manifest of the remote artifact
// - verifies that artifact config matches Timoni's media type
// - downloads all the compressed layer matching Timoni's media type (if not cached)
// - stores the compressed layers in the local cache (if caching is enabled)
// - extracts the module contents to the destination directory
func PullModule(ociURL, dstPath, cacheDir string, opts []crane.Option) (*apiv1.ModuleReference, error) {
	ref, err := parseArtifactRef(ociURL)
	if err != nil {
		return nil, err
	}

	repoURL := ref.Context().Name()

	digest, err := crane.Digest(ref.String(), opts...)
	if err != nil {
		return nil, fmt.Errorf("resolving digest of '%s' failed: %w", ociURL, err)
	}

	manifestJSON, err := crane.Manifest(ref.String(), opts...)
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

	version := ""
	if rev, ok := manifest.Annotations[apiv1.RevisionAnnotation]; ok == true {
		// For backwards compatibility with Timoni v0.13
		version = rev
	}
	if ver, ok := manifest.Annotations[apiv1.VersionAnnotation]; ok == true {
		version = ver
	}

	moduleRef := &apiv1.ModuleReference{
		Repository: fmt.Sprintf("%s%s", apiv1.ArtifactPrefix, repoURL),
		Version:    version,
		Digest:     digest,
	}

	// If caching is disable, download the compressed layers to an ephemeral tmp dir.
	if cacheDir == "" {
		tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tmpDir)
		cacheDir = tmpDir
	}

	var foundLayer bool
	for _, layer := range manifest.Layers {
		if layer.MediaType == apiv1.ContentMediaType {
			foundLayer = true
			layerDigest := layer.Digest.String()
			blobURL := fmt.Sprintf("%s@%s", repoURL, layerDigest)

			isCached := false
			cachedLayer := path.Join(cacheDir, fmt.Sprintf("%s.tgz", layer.Digest.Hex))
			if _, err := os.Stat(cachedLayer); err == nil {
				isCached = true
			}

			// Pull the compressed layer from the registry and persist the gzip tarball
			// in the cache at '<cache-dir>/<layer-digest-hex>.tgz'.
			if !isCached {
				layer, err := crane.PullLayer(blobURL, opts...)
				if err != nil {
					return nil, fmt.Errorf("pulling layer %s failed: %w", layerDigest, err)
				}

				remote, err := layer.Compressed()
				if err != nil {
					return nil, fmt.Errorf("pulling layer %s failed: %w", layerDigest, err)
				}

				local, err := os.Create(cachedLayer)
				if err != nil {
					return nil, fmt.Errorf("writing layer to storage failed: %w", err)
				}

				if _, err := io.Copy(local, remote); err != nil {
					return nil, fmt.Errorf("writing layer to storage failed: %w", err)
				}

				if err := local.Close(); err != nil {
					return nil, fmt.Errorf("writing layer to storage failed: %w", err)
				}
			}

			reader, err := os.Open(cachedLayer)
			if err != nil {
				return nil, fmt.Errorf("reading layer from storage failed: %w", err)
			}

			// Extract the contents from the gzip tarball stored in cache.
			// If extraction fails, the gzip tarball is removed from cache.
			if err = tar.Untar(reader, dstPath, tar.WithMaxUntarSize(-1)); err != nil {
				_ = reader.Close()
				_ = os.Remove(cachedLayer)
				return nil, fmt.Errorf("extracting layer %s failed: %w", layerDigest, err)
			}

			if err := reader.Close(); err != nil {
				return nil, fmt.Errorf("reading layer from storage failed: %w", err)
			}
		}
	}

	if !foundLayer {
		return nil, fmt.Errorf("no layer found in artifact with media type '%s'", apiv1.ContentMediaType)
	}

	return moduleRef, nil
}
