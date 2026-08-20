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
	"regexp"
	"sort"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// ListArtifactOptions holds the options for listing the tags
// of an artifact repository.
type ListArtifactOptions struct {
	// WithDigest enables the resolving of the digest for each tag.
	WithDigest bool

	// FilterRegex is a regular expression used to filter the tags,
	// all tags are returned if empty.
	FilterRegex string

	// FilterSemver is a semantic version range used to filter the tags,
	// all tags are returned if empty.
	FilterSemver string

	// Limit caps the number of tags returned in descending lexical order,
	// all tags are returned if 0. The latest tag, when present,
	// is always included and does not count towards the limit.
	Limit int
}

// ListArtifactTags performs the following operations:
//   - fetches the digest of the latest tag (if it exists)
//   - lists all the tags from the artifact repository
//   - filters the tags based on the regex and semver expressions (if configured to do so)
//   - truncates the tags to the configured limit
//   - fetches the digest of each tag concurrently (if configured to do so)
//   - returns an array of ArtifactReference objects along with the total
//     number of tags found before the limit was applied (excluding latest)
func ListArtifactTags(ctx context.Context, ociURL string, listOpts ListArtifactOptions, opts []crane.Option) ([]apiv1.ArtifactReference, int, error) {
	var list []apiv1.ArtifactReference

	ref, err := parseArtifactRef(ociURL)
	if err != nil {
		return nil, 0, err
	}

	filter, err := newTagFilter(listOpts.FilterRegex, listOpts.FilterSemver)
	if err != nil {
		return nil, 0, err
	}

	withDigest := listOpts.WithDigest
	repoURL := ref.Context().Name()

	if filter.matches(name.DefaultTag) {
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
	}

	tags, err := crane.ListTags(repoURL, opts...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing tags failed: %w", err)
	}

	sort.Slice(tags, func(i, j int) bool { return tags[i] > tags[j] })

	filtered := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag == name.DefaultTag || !filter.matches(tag) {
			continue
		}
		filtered = append(filtered, tag)
	}
	total := len(filtered)
	tags = limitTags(filtered, listOpts.Limit)

	digests := make([]string, len(tags))
	if withDigest {
		digests, err = resolveDigests(ctx, repoURL, tags, opts)
		if err != nil {
			return nil, 0, err
		}
	}

	for i, tag := range tags {
		list = append(list, apiv1.ArtifactReference{
			Repository: ociURL,
			Tag:        tag,
			Digest:     digests[i],
		})
	}

	return list, total, nil
}

// tagFilter filters OCI tags by regular expression and semantic version range.
type tagFilter struct {
	regex      *regexp.Regexp
	constraint *semver.Constraints
}

// newTagFilter returns a tagFilter for the given regular expression and
// semantic version range, both expressions are optional.
func newTagFilter(filterRegex, filterSemver string) (*tagFilter, error) {
	f := &tagFilter{}

	if filterRegex != "" {
		regex, err := regexp.Compile(filterRegex)
		if err != nil {
			return nil, fmt.Errorf("invalid regex filter '%s': %w", filterRegex, err)
		}
		f.regex = regex
	}

	if filterSemver != "" {
		constraint, err := semver.NewConstraint(filterSemver)
		if err != nil {
			return nil, fmt.Errorf("invalid semver filter '%s': %w", filterSemver, err)
		}
		f.constraint = constraint
	}

	return f, nil
}

// matches returns true if the tag satisfies both the regular expression and
// the semantic version range. Tags that are not valid semantic versions are
// filtered out when a semver range is set.
func (f *tagFilter) matches(tag string) bool {
	if f.regex != nil && !f.regex.MatchString(tag) {
		return false
	}

	if f.constraint != nil {
		version, err := semver.NewVersion(tag)
		if err != nil {
			return false
		}
		if !f.constraint.Check(version) {
			return false
		}
	}

	return true
}
