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
	"context"
	"fmt"
	"sort"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// ListModuleOptions holds the options for listing the versions
// of a module repository.
type ListModuleOptions struct {
	// WithDigest enables the resolving of the digest for each version.
	WithDigest bool

	// Limit caps the number of versions returned, newest first,
	// all versions are returned if 0. The latest tag, when present,
	// is always included and does not count towards the limit.
	Limit int
}

// ListModuleVersions performs the following operations:
//   - lists all the tags from to this module repository
//   - filters and orders the tags based on semver
//   - truncates the versions to the configured limit
//   - fetches the digest of the latest version
//   - fetches the digest of each version concurrently (if configured to do so)
//   - returns an array of ModuleReference objects along with the total
//     number of versions found before the limit was applied (excluding latest)
func ListModuleVersions(ctx context.Context, ociURL string, listOpts ListModuleOptions, opts []crane.Option) ([]apiv1.ModuleReference, int, error) {
	var list []apiv1.ModuleReference
	withDigest := listOpts.WithDigest

	ref, err := parseArtifactRef(ociURL)
	if err != nil {
		return nil, 0, err
	}

	repoURL := ref.Context().Name()

	tags, err := crane.ListTags(repoURL, opts...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing tags failed: %w", err)
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

	tags = make([]string, 0, len(versions))
	for _, v := range versions {
		tags = append(tags, v.String())
	}
	total := len(tags)
	tags = limitTags(tags, listOpts.Limit)

	if digest, err := crane.Digest(fmt.Sprintf("%s:%s", repoURL, name.DefaultTag), opts...); err == nil {
		if !withDigest {
			digest = ""
		}
		list = append(list, apiv1.ModuleReference{
			Repository: ociURL,
			Version:    name.DefaultTag,
			Digest:     digest,
		})
	}

	digests := make([]string, len(tags))
	if withDigest {
		digests, err = resolveDigests(ctx, repoURL, tags, opts)
		if err != nil {
			return nil, 0, err
		}
	}

	for i, tag := range tags {
		list = append(list, apiv1.ModuleReference{
			Repository: ociURL,
			Version:    tag,
			Digest:     digests[i],
		})
	}

	return list, total, nil
}
