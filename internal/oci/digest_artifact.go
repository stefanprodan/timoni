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

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// GetArtifactDigest resolves the digest of the OpenContainers artifact
// found at the given URL. When the URL contains no tag, the 'latest'
// tag is used. The returned reference contains the canonical repository
// name, the resolved tag and the digest in the format '<sha-type>:<hex>'.
func GetArtifactDigest(ociURL string, opts []crane.Option) (apiv1.ArtifactReference, error) {
	ref, err := parseArtifactRef(ociURL)
	if err != nil {
		return apiv1.ArtifactReference{}, err
	}

	digest, err := crane.Digest(ref.Name(), opts...)
	if err != nil {
		return apiv1.ArtifactReference{}, fmt.Errorf("resolving digest of '%s' failed: %w", ociURL, err)
	}

	var tag string
	if t, ok := ref.(name.Tag); ok {
		tag = t.TagStr()
	}

	return apiv1.ArtifactReference{
		Repository: apiv1.ArtifactPrefix + ref.Context().Name(),
		Tag:        tag,
		Digest:     digest,
	}, nil
}
