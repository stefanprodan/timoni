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
	"os"
	"path/filepath"
	"slices"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"github.com/Masterminds/semver/v3"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// ModuleSourceLister extends module version lookup with verified local source
// resolution.
type ModuleSourceLister interface {
	ModuleVersionLister
	ResolveSource(ctx context.Context, repository, version string) (string, error)
}

type localIndexEntry struct {
	source string
	digest string
}

// LocalModuleIndexLister reads an explicit CUE index of local modules and falls
// back to another version provider for identities not present in the index.
// The index never derives a registry identity from a filesystem path.
type LocalModuleIndexLister struct {
	indexDir string
	entries  map[string]map[string]localIndexEntry
	aliases  map[string]string
	fallback ModuleVersionLister
}

// NewLocalModuleIndexLister loads a CUE index whose shape is:
//
//	modules: {
//	  "oci://registry.example/team/module": versions: {
//	    "1.2.3": {source: "file://modules/module-1.2.3", digest: "sha256:..."}
//	  }
//	}
//
// Relative sources are resolved relative to the index file.
func NewLocalModuleIndexLister(indexPath string, fallback ModuleVersionLister) (*LocalModuleIndexLister, error) {
	content, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read local module index %s: %w", indexPath, err)
	}
	indexPath, err = filepath.Abs(indexPath)
	if err != nil {
		return nil, err
	}
	file, err := parser.ParseFile(indexPath, content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse local module index: %w", err)
	}
	value := cuecontext.New().BuildFile(file)
	if err := value.Validate(); err != nil {
		return nil, fmt.Errorf("invalid local module index: %w", err)
	}

	lister := &LocalModuleIndexLister{
		indexDir: filepath.Dir(indexPath),
		entries:  make(map[string]map[string]localIndexEntry),
		aliases:  make(map[string]string),
		fallback: fallback,
	}
	modules := value.LookupPath(cue.ParsePath("modules"))
	iter, err := modules.Fields()
	if err != nil {
		return nil, fmt.Errorf("local module index modules must be a struct: %w", err)
	}
	for iter.Next() {
		identity := iter.Selector().Unquoted()
		if identity == "" || !strings.HasPrefix(identity, apiv1.ArtifactPrefix) {
			return nil, fmt.Errorf("local module index identity %q must use oci://", identity)
		}
		versions := iter.Value().LookupPath(cue.ParsePath("versions"))
		versionIter, err := versions.Fields()
		if err != nil {
			return nil, fmt.Errorf("local module index %s versions must be a struct: %w", identity, err)
		}
		if _, exists := lister.entries[identity]; exists {
			return nil, fmt.Errorf("duplicate local module index identity %s", identity)
		}
		lister.entries[identity] = make(map[string]localIndexEntry)
		for versionIter.Next() {
			version := versionIter.Selector().Unquoted()
			if _, err := semver.StrictNewVersion(version); err != nil {
				return nil, fmt.Errorf("local module index %s has invalid semantic version %q", identity, version)
			}
			entryValue := versionIter.Value()
			source, err := entryValue.LookupPath(cue.ParsePath("source")).String()
			if err != nil || source == "" {
				return nil, fmt.Errorf("local module index %s version %s has no source", identity, version)
			}
			digest, err := entryValue.LookupPath(cue.ParsePath("digest")).String()
			if err != nil || !validSHA256Digest(digest) {
				return nil, fmt.Errorf("local module index %s version %s has invalid SHA-256 digest", identity, version)
			}
			canonical, err := canonicalLocalSource(source, lister.indexDir)
			if err != nil {
				return nil, fmt.Errorf("local module index %s version %s: %w", identity, version, err)
			}
			if previous, exists := lister.aliases[canonical]; exists && previous != identity {
				return nil, fmt.Errorf("local module source %s is assigned to both %s and %s", canonical, previous, identity)
			}
			lister.aliases[canonical] = identity
			lister.entries[identity][version] = localIndexEntry{source: canonical, digest: digest}
		}
	}
	return lister, nil
}

// ResolveIdentity maps a bundle repository to an indexed module identity.
// Local source URLs are resolved relative to the bundle file that declared
// them; OCI identities are matched exactly.
func (l *LocalModuleIndexLister) ResolveIdentity(repository, baseDir string) (string, bool, error) {
	if strings.HasPrefix(repository, apiv1.LocalPrefix) {
		canonical, err := canonicalLocalSource(repository, baseDir)
		if err != nil {
			return "", false, err
		}
		identity, ok := l.aliases[canonical]
		return identity, ok, nil
	}
	_, ok := l.entries[repository]
	return repository, ok, nil
}

// ListVersions lists indexed semantic versions or delegates to the fallback.
func (l *LocalModuleIndexLister) ListVersions(ctx context.Context, repository string) ([]string, error) {
	entries, ok := l.entries[repository]
	if !ok {
		if l.fallback == nil {
			return nil, fmt.Errorf("module %s is not in the local index", repository)
		}
		return l.fallback.ListVersions(ctx, repository)
	}
	versions := make([]string, 0, len(entries))
	for version := range entries {
		versions = append(versions, version)
	}
	slices.SortFunc(versions, func(a, b string) int {
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
	return versions, nil
}

// ResolveDigest returns the digest recorded for an indexed version.
func (l *LocalModuleIndexLister) ResolveDigest(ctx context.Context, repository, version string) (string, error) {
	entries, ok := l.entries[repository]
	if !ok {
		if l.fallback == nil {
			return "", fmt.Errorf("module %s is not in the local index", repository)
		}
		return l.fallback.ResolveDigest(ctx, repository, version)
	}
	entry, ok := entries[version]
	if !ok {
		return "", fmt.Errorf("version %s is not in the local index for %s", version, repository)
	}
	return entry.digest, nil
}

// ResolveSource verifies and returns the local source recorded for an indexed
// version.
func (l *LocalModuleIndexLister) ResolveSource(_ context.Context, repository, version string) (string, error) {
	entries, ok := l.entries[repository]
	if !ok {
		return "", nil
	}
	entry, ok := entries[version]
	if !ok {
		return "", fmt.Errorf("version %s is not in the local index for %s", version, repository)
	}
	actual, err := ModuleSourceDigest(strings.TrimPrefix(entry.source, apiv1.LocalPrefix))
	if err != nil {
		return "", fmt.Errorf("verify local module %s version %s: %w", repository, version, err)
	}
	if actual != entry.digest {
		return "", fmt.Errorf("local module %s version %s digest mismatch: index has %s, source has %s", repository, version, entry.digest, actual)
	}
	return entry.source, nil
}

func canonicalLocalSource(source, baseDir string) (string, error) {
	if !strings.HasPrefix(source, apiv1.LocalPrefix) {
		return "", fmt.Errorf("source %q must use file://", source)
	}
	pathValue := strings.TrimPrefix(source, apiv1.LocalPrefix)
	if pathValue == "" {
		return "", fmt.Errorf("source is empty")
	}
	if !filepath.IsAbs(pathValue) {
		pathValue = filepath.Join(baseDir, pathValue)
	}
	abs, err := filepath.Abs(pathValue)
	if err != nil {
		return "", err
	}
	return apiv1.LocalPrefix + filepath.Clean(abs), nil
}
