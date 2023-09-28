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

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// ListModuleVersions performs the following operations:
// - lists all the tags from to this module repository
// - filters and orders the tags based on semver
// - fetches the digest of the latest version
// - fetches the digest of each version (if configured to do so)
// - returns an array of ModuleReference objects
func ListModuleVersions(ociURL string, withDigest bool, opts []crane.Option) ([]apiv1.ModuleReference, error) {
	var list []apiv1.ModuleReference

	ref, err := parseArtifactRef(ociURL)
	if err != nil {
		return nil, err
	}

	repoURL := ref.Context().Name()

	tags, err := crane.ListTags(repoURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("listing tags failed: %w", err)
	}

	var versions []*semver.Version
	for _, tag := range tags {
		if v, err := semver.StrictNewVersion(tag); err != nil {
			continue
		} else {
			versions = append(versions, v)
		}
	}
	sort.Sort(sort.Reverse(semver.Collection(versions)))

	if digest, err := crane.Digest(fmt.Sprintf("%s:%s", repoURL, name.DefaultTag), opts...); err == nil {
		list = append(list, apiv1.ModuleReference{
			Repository: ociURL,
			Version:    name.DefaultTag,
			Digest:     digest,
		})
	}

	for _, v := range versions {
		digest := ""
		if withDigest {
			d, err := crane.Digest(fmt.Sprintf("%s:%s", repoURL, v.String()), opts...)
			if err != nil {
				return nil, fmt.Errorf("faild to get digest for '%s': %w", v.String(), err)
			}
			digest = d
		}
		list = append(list, apiv1.ModuleReference{
			Repository: ociURL,
			Version:    v.String(),
			Digest:     digest,
		})

	}

	return list, nil
}
