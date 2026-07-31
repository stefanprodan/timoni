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
	"path/filepath"

	"github.com/fluxcd/pkg/tar"
	"github.com/google/go-containerregistry/pkg/crane"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	godigest "github.com/opencontainers/go-digest"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// PullModule performs the following operations:
// - determines the artifact digest corresponding to the module version
// - fetches the manifest of the remote artifact
// - verifies that artifact config matches Timoni's media type
// - downloads all compressed layers matching Timoni's media type (if not cached)
// - atomically stores the compressed layers in the local cache (if caching is enabled)
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
	if rev, ok := manifest.Annotations[apiv1.RevisionAnnotation]; ok {
		// For backwards compatibility with Timoni v0.13
		version = rev
	}
	if ver, ok := manifest.Annotations[apiv1.VersionAnnotation]; ok {
		version = ver
	}

	moduleRef := &apiv1.ModuleReference{
		Repository:  fmt.Sprintf("%s%s", apiv1.ArtifactPrefix, repoURL),
		Version:     version,
		Digest:      digest,
		Annotations: manifest.Annotations,
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

			cachedLayer := filepath.Join(cacheDir, fmt.Sprintf("%s.tgz", layer.Digest.Hex))
			expectedDigest, err := godigest.Parse(layerDigest)
			if err != nil {
				return nil, fmt.Errorf("parsing layer digest %s failed: %w", layerDigest, err)
			}

			isCached, err := verifyCachedLayer(cachedLayer, expectedDigest)
			if err != nil {
				return nil, fmt.Errorf("reading layer from storage failed: %w", err)
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

				if err := writeCachedLayer(cachedLayer, remote, expectedDigest); err != nil {
					return nil, fmt.Errorf("writing layer to storage failed: %w", err)
				}
			}

			reader, err := os.Open(cachedLayer)
			if err != nil {
				return nil, fmt.Errorf("reading layer from storage failed: %w", err)
			}

			// Extract the contents from the gzip tarball stored in cache.
			if err = tar.Untar(reader, dstPath, tar.WithMaxUntarSize(-1)); err != nil {
				_ = reader.Close()
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

// verifyCachedLayer reports whether cachedLayer matches expectedDigest.
func verifyCachedLayer(cachedLayer string, expectedDigest godigest.Digest) (bool, error) {
	local, err := os.Open(cachedLayer)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer local.Close()

	verifier := expectedDigest.Verifier()
	if _, err := io.Copy(verifier, local); err != nil {
		return false, err
	}
	return verifier.Verified(), nil
}

// writeCachedLayer verifies and publishes a complete compressed layer at cachedLayer.
func writeCachedLayer(cachedLayer string, remote io.Reader, expectedDigest godigest.Digest) error {
	local, err := os.CreateTemp(filepath.Dir(cachedLayer), ".layer-*")
	if err != nil {
		return err
	}
	temporaryLayer := local.Name()
	defer func() {
		_ = os.Remove(temporaryLayer)
	}()

	verifier := expectedDigest.Verifier()
	if _, err := io.Copy(io.MultiWriter(local, verifier), remote); err != nil {
		_ = local.Close()
		return err
	}
	if !verifier.Verified() {
		_ = local.Close()
		return fmt.Errorf("layer digest mismatch: expected %s", expectedDigest)
	}
	if err := local.Close(); err != nil {
		return err
	}

	if err := os.Rename(temporaryLayer, cachedLayer); err != nil {
		if valid, verifyErr := verifyCachedLayer(cachedLayer, expectedDigest); verifyErr == nil && valid {
			return nil
		}
		return err
	}
	return nil
}
