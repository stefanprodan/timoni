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

package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/oci"
)

type localArtifactEntry struct {
	source string
	digest string
}

// LocalModuleArtifactLister adds explicitly mapped local OCI artifacts to
// module version lookup and delegates unmapped versions to its fallback provider.
type LocalModuleArtifactLister struct {
	entries  map[string]map[string]localArtifactEntry
	aliases  map[string]string
	fallback ModuleVersionLister
}

// NewLocalModuleArtifactLister loads mappings of the form
// "oci://registry.example/team/module=/path/to/module.oci.tar". The mapping
// is explicit because local OCI metadata does not contain the repository name.
func NewLocalModuleArtifactLister(mappings []string, fallback ModuleVersionLister) (*LocalModuleArtifactLister, error) {
	lister := &LocalModuleArtifactLister{
		entries:  make(map[string]map[string]localArtifactEntry),
		aliases:  make(map[string]string),
		fallback: fallback,
	}
	for _, mapping := range mappings {
		repository, source, ok := strings.Cut(mapping, "=")
		if !ok || repository == apiv1.ArtifactPrefix || !strings.HasPrefix(repository, apiv1.ArtifactPrefix) || source == "" {
			return nil, fmt.Errorf("invalid local OCI mapping %q, expected oci://repository=path", mapping)
		}
		source = strings.TrimPrefix(source, apiv1.LocalPrefix)
		abs, err := filepath.Abs(source)
		if err != nil {
			return nil, fmt.Errorf("resolving local OCI artifact %s failed: %w", source, err)
		}
		version, digest, err := oci.LocalModuleMetadata(abs)
		if err != nil {
			return nil, fmt.Errorf("reading local OCI artifact %s failed: %w", abs, err)
		}
		if _, err := semver.StrictNewVersion(version); err != nil {
			return nil, fmt.Errorf("local OCI artifact %s has invalid semantic version %q", abs, version)
		}
		canonical := apiv1.LocalPrefix + filepath.Clean(abs)
		if previous, exists := lister.aliases[canonical]; exists && previous != repository {
			return nil, fmt.Errorf("local OCI artifact %s is assigned to both %s and %s", canonical, previous, repository)
		}
		if previous, exists := lister.entries[repository][version]; exists {
			if previous.digest != digest || previous.source != canonical {
				return nil, fmt.Errorf("local OCI artifact %s version %s conflicts with another artifact", repository, version)
			}
		}
		if lister.entries[repository] == nil {
			lister.entries[repository] = make(map[string]localArtifactEntry)
		}
		lister.entries[repository][version] = localArtifactEntry{source: canonical, digest: digest}
		lister.aliases[canonical] = repository
	}
	return lister, nil
}

// HasRepository reports whether a local artifact is mapped to the repository.
func (l *LocalModuleArtifactLister) HasRepository(repository string) bool {
	_, ok := l.entries[repository]
	return ok
}

// ResolveIdentity maps a local artifact source to its explicit OCI repository.
func (l *LocalModuleArtifactLister) ResolveIdentity(repository, baseDir string) (string, bool, error) {
	if !strings.HasPrefix(repository, apiv1.LocalPrefix) {
		_, ok := l.entries[repository]
		return repository, ok, nil
	}
	canonical, err := canonicalLocalSource(repository, baseDir)
	if err != nil {
		return "", false, err
	}
	identity, ok := l.aliases[canonical]
	return identity, ok, nil
}

// ListVersions returns the union of local artifact versions and fallback
// versions, sorted newest first.
func (l *LocalModuleArtifactLister) ListVersions(ctx context.Context, repository string) ([]string, error) {
	versions := make(map[string]struct{})
	for version := range l.entries[repository] {
		versions[version] = struct{}{}
	}
	if l.fallback != nil {
		fallback, err := l.fallback.ListVersions(ctx, repository)
		if err != nil && len(versions) == 0 {
			return nil, err
		}
		for _, version := range fallback {
			versions[version] = struct{}{}
		}
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("module %s is not in the local OCI inputs", repository)
	}
	result := make([]string, 0, len(versions))
	for version := range versions {
		result = append(result, version)
	}
	slices.SortFunc(result, func(a, b string) int {
		av, _ := semver.StrictNewVersion(a)
		bv, _ := semver.StrictNewVersion(b)
		if av.GreaterThan(bv) {
			return -1
		}
		if av.LessThan(bv) {
			return 1
		}
		return 0
	})
	return result, nil
}

// ResolveDigest returns the verified digest of a local artifact or delegates
// to the fallback provider for a version that is not mapped locally.
func (l *LocalModuleArtifactLister) ResolveDigest(ctx context.Context, repository, version string) (string, error) {
	if entry, ok := l.entries[repository][version]; ok {
		if err := l.verify(entry, repository, version); err != nil {
			return "", err
		}
		return entry.digest, nil
	}
	if l.fallback == nil {
		return "", fmt.Errorf("version %s is not in the local OCI inputs for %s", version, repository)
	}
	return l.fallback.ResolveDigest(ctx, repository, version)
}

// ResolveSource returns the verified local artifact source for a mapped
// version, or delegates to the fallback provider.
func (l *LocalModuleArtifactLister) ResolveSource(ctx context.Context, repository, version string) (string, error) {
	if entry, ok := l.entries[repository][version]; ok {
		if err := l.verify(entry, repository, version); err != nil {
			return "", err
		}
		return entry.source, nil
	}
	if resolver, ok := l.fallback.(ModuleSourceLister); ok {
		return resolver.ResolveSource(ctx, repository, version)
	}
	return "", nil
}

func (l *LocalModuleArtifactLister) verify(entry localArtifactEntry, repository, version string) error {
	actualVersion, digest, err := oci.LocalModuleMetadata(strings.TrimPrefix(entry.source, apiv1.LocalPrefix))
	if err != nil {
		return fmt.Errorf("verify local OCI artifact %s version %s failed: %w", repository, version, err)
	}
	if actualVersion != version {
		return fmt.Errorf("local OCI artifact %s version mismatch: input has %s, artifact has %s", repository, version, actualVersion)
	}
	if digest != entry.digest {
		return fmt.Errorf("local OCI artifact %s version %s digest mismatch: input has %s, artifact has %s", repository, version, entry.digest, digest)
	}
	return nil
}
