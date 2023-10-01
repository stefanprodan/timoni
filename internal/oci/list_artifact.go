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
	"sort"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// ListArtifactTags performs the following operations:
// - fetches the digest of the latest tag (if it exists)
// - lists all the tags from the artifact repository
// - fetches the digest of each tag (if configured to do so)
// - returns an array of ArtifactReference objects
func ListArtifactTags(ociURL string, withDigest bool, opts []crane.Option) ([]apiv1.ArtifactReference, error) {
	var list []apiv1.ArtifactReference

	ref, err := parseArtifactRef(ociURL)
	if err != nil {
		return nil, err
	}

	repoURL := ref.Context().Name()

	if digest, err := crane.Digest(fmt.Sprintf("%s:%s", repoURL, name.DefaultTag), opts...); err == nil {
		if !withDigest {
			digest = ""
		}
		list = append(list, apiv1.ArtifactReference{
			Repository: ociURL,
			Tag:        name.DefaultTag,
			Digest:     digest,
		})
	}

	tags, err := crane.ListTags(repoURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("listing tags failed: %w", err)
	}

	sort.Slice(tags, func(i, j int) bool { return tags[i] > tags[j] })

	for _, tag := range tags {
		if tag == name.DefaultTag {
			continue
		}
		digest := ""
		if withDigest {
			d, err := crane.Digest(fmt.Sprintf("%s:%s", repoURL, tag), opts...)
			if err != nil {
				return nil, fmt.Errorf("faild to get digest for '%s': %w", tag, err)
			}
			digest = d
		}
		list = append(list, apiv1.ArtifactReference{
			Repository: ociURL,
			Tag:        tag,
			Digest:     digest,
		})
	}

	return list, nil
}
