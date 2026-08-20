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
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/crane"
	"golang.org/x/sync/errgroup"
)

// digestConcurrency is the maximum number of digest requests
// issued in parallel when listing tags.
const digestConcurrency = 8

// resolveDigests fetches the manifest digest of each tag concurrently
// and returns the digests in the same order as the tags.
// The first failure cancels the pending requests and is returned
// with the tag that caused it.
func resolveDigests(ctx context.Context, repoURL string, tags []string, opts []crane.Option) ([]string, error) {
	digests := make([]string, len(tags))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(digestConcurrency)
	opts = append(append([]crane.Option{}, opts...), crane.WithContext(ctx))

	for i, tag := range tags {
		g.Go(func() error {
			d, err := crane.Digest(fmt.Sprintf("%s:%s", repoURL, tag), opts...)
			if err != nil {
				return fmt.Errorf("failed to get digest for '%s': %w", tag, err)
			}
			digests[i] = d
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return digests, nil
}

// limitTags returns the first limit tags, or all tags when limit is 0.
func limitTags(tags []string, limit int) []string {
	if limit > 0 && len(tags) > limit {
		return tags[:limit]
	}
	return tags
}
