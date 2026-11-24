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
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/gomega"
)

// fakeVersionLister serves the module versions of a repository from memory,
// derives the digest of a version from its name, and records the
// repositories and the versions it was asked for.
type fakeVersionLister struct {
	versions map[string][]string
	calls    []string
	digests  []string
}

func (l *fakeVersionLister) ListVersions(_ context.Context, repository string) ([]string, error) {
	l.calls = append(l.calls, repository)
	list, ok := l.versions[repository]
	if !ok {
		return nil, fmt.Errorf("repository not found")
	}
	return list, nil
}

func (l *fakeVersionLister) ResolveDigest(_ context.Context, repository, version string) (string, error) {
	l.digests = append(l.digests, repository+":"+version)
	if !slices.Contains(l.versions[repository], version) {
		return "", fmt.Errorf("version not found")
	}
	return "sha256:" + version, nil
}

func versions(versions ...string) []string {
	return versions
}

func writeBundleFiles(t *testing.T, files map[string]string) []string {
	t.Helper()
	dir := t.TempDir()
	paths := make([]string, 0, len(files))
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	return paths
}

func changeFor(plan *UpdatePlan, instance string) *UpdateChange {
	for _, c := range plan.Changes {
		for _, name := range c.Instances {
			if name == instance {
				return c
			}
		}
	}
	return nil
}

func skipFor(plan *UpdatePlan, instance string) *UpdateSkip {
	for _, s := range plan.Skipped {
		if s.Instance == instance {
			return s
		}
	}
	return nil
}

func indexOf(paths []string, name string) int {
	for i, p := range paths {
		if filepath.Base(p) == name {
			return i
		}
	}
	return -1
}

func cuePath(p string) cue.Path {
	return cue.ParsePath(p)
}

func cueString(v cue.Value) string {
	s, _ := v.String()
	return s
}

func TestBundleUpdater_Plan(t *testing.T) {
	g := NewWithT(t)

	const (
		podinfo = "oci://registry.local/modules/podinfo"
		redis   = "oci://registry.local/modules/redis"
		nginx   = "oci://registry.local/modules/nginx"
	)

	bundle := `
_versions: podinfo: "6.1.0" @timoni(update:semver:6.x)

let redisVer = "8.0.0"

bundle: {
	apiVersion: "v1alpha1"
	name:       "test"
	instances: {
		direct: {
			module: {
				url:     "` + podinfo + `"
				version: "6.0.0" @timoni(update:semver:>=6.0.0 <6.2.0)
				digest:  "sha256:6.0.0"
			}
			namespace: "test"
			values: {}
		}
		oneline: {
			module: url:     "` + redis + `"
			module: version: "8.0.0" @timoni(update:semver:8.x)
			namespace: "test"
			values: {}
		}
		"shared-a": {
			module: url:     "` + podinfo + `"
			module: version: _versions.podinfo
			namespace: "test"
			values: {}
		}
		"shared-b": {
			module: url:     "` + podinfo + `"
			module: version: _versions.podinfo
			module: digest:  "sha256:6.1.0"
			namespace: "test"
			values: {}
		}
		"shared-c": {
			module: url:     "` + podinfo + `"
			module: version: _versions.podinfo
			namespace: "test"
			values: {}
		}
		letref: {
			module: url:     "` + redis + `"
			module: version: redisVer @timoni(update:semver:*)
			namespace: "test"
			values: {}
		}
		floating: {
			module: url:    "` + redis + `"
			module: digest: "sha256:old" @timoni(update:digest)
			namespace: "test"
			values: {}
		}
		frozen: {
			module: url:     "` + redis + `"
			module: version: "7.0.0" @timoni(update:none)
			namespace: "test"
			values: {}
		}
		unmarked: {
			module: url:     "` + redis + `"
			module: version: "7.0.0"
			namespace: "test"
			values: {}
		}
		interpolated: {
			module: url:     "` + redis + `"
			module: version: "\(_major).0.0" @timoni(update:semver:*)
			namespace: "test"
			values: {}
		}
		unset: {
			module: url: "` + redis + `"
			namespace: "test"
			values: {}
		}
		local: {
			module: url:     "file://./modules/redis"
			module: version: "1.0.0" @timoni(update:semver:*)
			namespace: "test"
			values: {}
		}
		"in-partial": {
			module: url: "` + nginx + `"
			namespace: "test"
			values: {}
		}
		"in-other-file": {
			module: url: "` + nginx + `"
			namespace: "test"
			values: {}
		}
		"up-to-date": {
			module: url:     "` + redis + `"
			module: version: "8.1.0" @timoni(update:semver:8.x)
			module: digest:  "sha256:8.1.0"
			namespace: "test"
			values: {}
		}
	}
}

_major: 7
`
	partial := `
bundle:
  instances:
    in-partial:
      module:
        version: "1.0.0"
`
	other := `
bundle: instances: "in-other-file": module: version: "1.0.0" @timoni(update:semver:*)
bundle: instances: "shared-c": module: digest: "sha256:6.1.0"
`

	files := writeBundleFiles(t, map[string]string{
		"bundle.cue":   bundle,
		"partial.yaml": partial,
		"other.cue":    other,
	})

	lister := &fakeVersionLister{versions: map[string][]string{
		podinfo: append(versions("latest"), versions("7.0.0", "6.2.0", "6.1.5", "6.1.0", "6.0.0")...),
		redis:   append(versions("latest"), versions("8.1.0", "8.0.0", "7.0.0")...),
		nginx:   versions("2.0.0", "1.0.0"),
	}}

	updater := NewBundleUpdater(cuecontext.New(), files)
	g.Expect(updater.Load()).To(Succeed())

	plan, err := updater.Plan(context.Background(), lister)
	g.Expect(err).ToNot(HaveOccurred())

	// Each repository is listed once regardless of how many instances reference it,
	// and only the digests of the selected versions pinned by an instance are resolved.
	g.Expect(lister.calls).To(ConsistOf(podinfo, redis, nginx))
	g.Expect(lister.digests).To(ConsistOf(podinfo+":6.1.5", podinfo+":6.2.0", redis+":latest", redis+":8.1.0"))

	direct := changeFor(plan, "direct")
	g.Expect(direct).ToNot(BeNil())
	g.Expect(direct.ToVersion).To(Equal("6.1.5"))
	g.Expect(direct.FromDigest).To(Equal("sha256:6.0.0"))
	g.Expect(direct.ToDigest).To(Equal("sha256:6.1.5"))
	g.Expect(direct.Files).To(HaveExactElements(HaveSuffix("bundle.cue")))

	oneline := changeFor(plan, "oneline")
	g.Expect(oneline).ToNot(BeNil())
	g.Expect(oneline.ToVersion).To(Equal("8.1.0"))
	g.Expect(oneline.ToDigest).To(BeEmpty())

	// Instances sharing a literal are updated together with the policy declared on the literal.
	shared := changeFor(plan, "shared-a")
	g.Expect(shared).ToNot(BeNil())
	g.Expect(shared.Instances).To(ConsistOf("shared-a", "shared-b", "shared-c"))
	g.Expect(shared.FromVersion).To(Equal("6.1.0"))
	g.Expect(shared.ToVersion).To(Equal("6.2.0"))
	g.Expect(shared.ToDigest).To(Equal("sha256:6.2.0"))
	g.Expect(shared.Files).To(HaveExactElements(HaveSuffix("bundle.cue"), HaveSuffix("other.cue")))

	letref := changeFor(plan, "letref")
	g.Expect(letref).ToNot(BeNil())
	g.Expect(letref.ToVersion).To(Equal("8.1.0"))

	floating := changeFor(plan, "floating")
	g.Expect(floating).ToNot(BeNil())
	g.Expect(floating.FromVersion).To(Equal("latest"))
	g.Expect(floating.ToVersion).To(Equal("latest"))
	g.Expect(floating.ToDigest).To(Equal("sha256:latest"))

	other2 := changeFor(plan, "in-other-file")
	g.Expect(other2).ToNot(BeNil())
	g.Expect(other2.ToVersion).To(Equal("2.0.0"))
	g.Expect(other2.Files).To(HaveExactElements(HaveSuffix("other.cue")))

	g.Expect(skipFor(plan, "frozen").Reason).To(ContainSubstring("none"))
	g.Expect(skipFor(plan, "unmarked").Reason).To(ContainSubstring("none"))
	g.Expect(skipFor(plan, "interpolated").Reason).To(ContainSubstring("string literal"))
	g.Expect(skipFor(plan, "unset").Reason).To(ContainSubstring("not set"))
	g.Expect(skipFor(plan, "local").Reason).To(ContainSubstring("OCI"))
	g.Expect(skipFor(plan, "in-partial").Reason).To(ContainSubstring("CUE file"))
	g.Expect(skipFor(plan, "up-to-date").Reason).To(Equal("up to date"))

	g.Expect(plan.Changes).To(HaveLen(6))

	// Apply the changes and verify the files are rewritten and still valid.
	g.Expect(updater.Apply(plan)).To(Succeed())

	out, err := updater.Format(files[indexOf(files, "bundle.cue")])
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(out)).To(ContainSubstring(`podinfo: "6.2.0" @timoni(update:semver:6.x)`))
	g.Expect(string(out)).To(ContainSubstring(`let redisVer = "8.1.0"`))
	g.Expect(string(out)).To(ContainSubstring(`version: "6.1.5" @timoni(update:semver:>=6.0.0 <6.2.0)`))
	g.Expect(string(out)).To(ContainSubstring(`digest:  "sha256:6.1.5"`))
	g.Expect(string(out)).To(ContainSubstring(`digest:  "sha256:6.2.0"`))
	g.Expect(string(out)).To(ContainSubstring(`digest: "sha256:latest" @timoni(update:digest)`))
	g.Expect(string(out)).To(ContainSubstring(`version: "7.0.0" @timoni(update:none)`))

	out, err = updater.Format(files[indexOf(files, "other.cue")])
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(out)).To(ContainSubstring(`version: "2.0.0" @timoni(update:semver:*)`))
	g.Expect(string(out)).To(MatchRegexp(`digest:\s+"sha256:6\.2\.0"`))

	// The updated bundle evaluates to the new references.
	v := updater.Value().LookupPath(cuePath(`bundle.instances."shared-b".module`))
	g.Expect(v.LookupPath(cuePath("version"))).To(WithTransform(cueString, Equal("6.2.0")))
	g.Expect(v.LookupPath(cuePath("digest"))).To(WithTransform(cueString, Equal("sha256:6.2.0")))

	// A second plan finds everything up to date.
	plan, err = updater.Plan(context.Background(), lister)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(plan.Changes).To(BeEmpty())
}

func TestBundleUpdater_Level(t *testing.T) {
	const redis = "oci://registry.local/modules/redis"
	lister := &fakeVersionLister{versions: map[string][]string{
		redis: append(versions("latest"), versions("9.0.0", "8.2.0", "8.1.3", "8.1.0", "8.0.0", "1.0.0", "0.3.0", "0.2.1", "0.2.0", "0.1.0")...),
	}}
	bundleFor := func(module string) string {
		return `
bundle: {
	apiVersion: "v1alpha1"
	name:       "test"
	instances: redis: {
		module: url: "` + redis + `"
		module: ` + module + `
		namespace: "test"
		values: {}
	}
}
`
	}

	tests := []struct {
		name    string
		module  string
		level   string
		version string
		digest  string
		skip    string
	}{
		{name: "none leaves the reference alone", module: `version: "8.1.0"`, level: UpdateLevelNone, skip: "update policy is none"},
		{name: "patch picks the newest patch", module: `version: "8.1.0"`, level: UpdateLevelPatch, version: "8.1.3"},
		{name: "minor picks the newest minor", module: `version: "8.1.0"`, level: UpdateLevelMinor, version: "8.2.0"},
		{name: "major picks the newest version", module: `version: "8.1.0"`, level: UpdateLevelMajor, version: "9.0.0"},
		{name: "minor spans the zero major", module: `version: "0.1.0"`, level: UpdateLevelMinor, version: "0.3.0"},
		{name: "patch keeps the zero minor", module: `version: "0.2.0"`, level: UpdateLevelPatch, version: "0.2.1"},
		{name: "major on latest picks the newest", module: `version: "latest"`, level: UpdateLevelMajor, version: "9.0.0"},
		{name: "minor cannot apply to latest", module: `version: "latest"`, level: UpdateLevelMinor, skip: "not a semantic version"},
		{name: "digest only refreshes with any level", module: `digest: "sha256:old"`, level: UpdateLevelPatch, version: "latest", digest: "sha256:latest"},
		{name: "digest only is left alone without a level", module: `digest: "sha256:old"`, level: UpdateLevelNone, skip: "update policy is none"},
		{name: "digest policy refreshes a non-literal version", module: `{version: "8." + "1.0", digest: "sha256:old" @timoni(update:digest)}`, level: UpdateLevelNone, version: "8.1.0", digest: "sha256:8.1.0"},
		{name: "level cannot apply to a non-literal version", module: `{version: "8." + "1.0", digest: "sha256:old"}`, level: UpdateLevelMinor, skip: "module version is not a string literal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			files := writeBundleFiles(t, map[string]string{"bundle.cue": bundleFor(tt.module)})
			updater := NewBundleUpdater(cuecontext.New(), files)
			g.Expect(updater.SetLevel(tt.level)).To(Succeed())
			g.Expect(updater.Load()).To(Succeed())

			plan, err := updater.Plan(context.Background(), lister)
			g.Expect(err).ToNot(HaveOccurred())
			if tt.skip != "" {
				g.Expect(plan.Changes).To(BeEmpty())
				g.Expect(skipFor(plan, "redis").Reason).To(ContainSubstring(tt.skip))
				return
			}
			g.Expect(plan.Changes).To(HaveLen(1))
			g.Expect(plan.Changes[0].ToVersion).To(Equal(tt.version))
			g.Expect(plan.Changes[0].ToDigest).To(Equal(tt.digest))
		})
	}

	t.Run("rejects unknown level", func(t *testing.T) {
		g := NewWithT(t)
		updater := NewBundleUpdater(cuecontext.New(), nil)
		g.Expect(updater.SetLevel("nightly")).To(MatchError(ContainSubstring("unknown update level")))
	})
}

func TestBundleUpdater_Errors(t *testing.T) {
	const (
		podinfo = "oci://registry.local/modules/podinfo"
		redis   = "oci://registry.local/modules/redis"
	)
	lister := &fakeVersionLister{versions: map[string][]string{
		podinfo: versions("6.2.0", "6.1.0"),
		redis:   versions("8.1.0", "8.0.0"),
	}}

	tests := []struct {
		name     string
		bundle   string
		matchErr string
	}{
		{
			name:     "shared literal with different modules",
			matchErr: "reference different modules",
			bundle: `
_ver: "6.1.0" @timoni(update:semver:*)
bundle: {
	apiVersion: "v1alpha1"
	name:       "test"
	instances: {
		a: {
			module: url:     "` + podinfo + `"
			module: version: _ver
			namespace: "test"
			values: {}
		}
		b: {
			module: url:     "` + redis + `"
			module: version: _ver
			namespace: "test"
			values: {}
		}
	}
}
`,
		},
		{
			name:     "shared literal with conflicting policies",
			matchErr: "different update policies",
			bundle: `
_ver: "6.1.0"
bundle: {
	apiVersion: "v1alpha1"
	name:       "test"
	instances: {
		a: {
			module: url:     "` + podinfo + `"
			module: version: _ver @timoni(update:semver:6.x)
			namespace: "test"
			values: {}
		}
		b: {
			module: url:     "` + podinfo + `"
			module: version: _ver @timoni(update:none)
			namespace: "test"
			values: {}
		}
	}
}
`,
		},
		{
			name:     "invalid attribute",
			matchErr: "invalid format",
			bundle: `
bundle: {
	apiVersion: "v1alpha1"
	name:       "test"
	instances: a: {
		module: url:     "` + podinfo + `"
		module: version: "6.1.0" @timoni(update:latest)
		namespace: "test"
		values: {}
	}
}
`,
		},
		{
			name:     "invalid semver constraint",
			matchErr: "invalid semver constraint",
			bundle: `
bundle: {
	apiVersion: "v1alpha1"
	name:       "test"
	instances: a: {
		module: url:     "` + podinfo + `"
		module: version: "6.1.0" @timoni(update:semver:six)
		namespace: "test"
		values: {}
	}
}
`,
		},
		{
			name:     "no version matching the constraint",
			matchErr: "no version of",
			bundle: `
bundle: {
	apiVersion: "v1alpha1"
	name:       "test"
	instances: a: {
		module: url:     "` + podinfo + `"
		module: version: "6.1.0" @timoni(update:semver:7.x)
		namespace: "test"
		values: {}
	}
}
`,
		},
		{
			name:     "unknown repository",
			matchErr: "repository not found",
			bundle: `
bundle: {
	apiVersion: "v1alpha1"
	name:       "test"
	instances: a: {
		module: url:     "oci://registry.local/modules/unknown"
		module: version: "6.1.0" @timoni(update:semver:*)
		namespace: "test"
		values: {}
	}
}
`,
		},
		{
			name:     "bare update attribute",
			matchErr: "invalid format",
			bundle: `
bundle: {
	apiVersion: "v1alpha1"
	name:       "test"
	instances: a: {
		module: url:     "` + podinfo + `"
		module: version: "6.1.0" @timoni(update)
		namespace: "test"
		values: {}
	}
}
`,
		},
		{
			name:     "semver policy without a version",
			matchErr: "module version is not set, the semver policy requires a version literal",
			bundle: `
bundle: {
	apiVersion: "v1alpha1"
	name:       "test"
	instances: a: {
		module: url:    "` + podinfo + `"
		module: digest: "sha256:old" @timoni(update:semver:*)
		namespace: "test"
		values: {}
	}
}
`,
		},
		{
			name:     "shared digest with different versions",
			matchErr: "share the module digest but not the module version",
			bundle: `
_digest: "sha256:6.1.0"
bundle: {
	apiVersion: "v1alpha1"
	name:       "test"
	instances: {
		a: {
			module: url:     "` + podinfo + `"
			module: version: "6.1.0" @timoni(update:semver:*)
			module: digest:  _digest
			namespace: "test"
			values: {}
		}
		b: {
			module: url:     "` + podinfo + `"
			module: version: "6.0.0" @timoni(update:semver:*)
			module: digest:  _digest
			namespace: "test"
			values: {}
		}
	}
}
`,
		},
		{
			name:     "current version no longer published",
			matchErr: "version 7.0.0 not found",
			bundle: `
bundle: {
	apiVersion: "v1alpha1"
	name:       "test"
	instances: a: {
		module: url:     "` + podinfo + `"
		module: version: "7.0.0" @timoni(update:semver:*)
		namespace: "test"
		values: {}
	}
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			files := writeBundleFiles(t, map[string]string{"bundle.cue": tt.bundle})
			updater := NewBundleUpdater(cuecontext.New(), files)
			g.Expect(updater.Load()).To(Succeed())
			_, err := updater.Plan(context.Background(), lister)
			g.Expect(err).To(MatchError(ContainSubstring(tt.matchErr)))
		})
	}
}

func TestBundleUpdater_RuntimeAttribute(t *testing.T) {
	g := NewWithT(t)
	const redis = "oci://registry.local/modules/redis"
	bundle := `
bundle: {
	apiVersion: "v1alpha1"
	name:       "test"
	instances: redis: {
		module: url:     "` + redis + `"
		module: version: string @timoni(runtime:string:REDIS_VERSION)
		namespace: "test"
		values: password: string @timoni(runtime:string:REDIS_PASSWORD)
	}
}
`
	files := writeBundleFiles(t, map[string]string{"bundle.cue": bundle})
	updater := NewBundleUpdater(cuecontext.New(), files)
	g.Expect(updater.Load()).To(Succeed())

	plan, err := updater.Plan(context.Background(), &fakeVersionLister{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(plan.Changes).To(BeEmpty())
	g.Expect(skipFor(plan, "redis").Reason).To(ContainSubstring("runtime value"))
}

func TestBundleUpdater_RuntimeManagedLiteral(t *testing.T) {
	g := NewWithT(t)
	const redis = "oci://registry.local/modules/redis"
	bundle := `
_ver: "1.0.0" @timoni(runtime:string:REDIS_VERSION)
_url: "` + redis + `" @timoni(runtime:string:REDIS_URL)
bundle: {
	apiVersion: "v1alpha1"
	name:       "test"
	instances: {
		version: {
			module: url:     "` + redis + `"
			module: version: _ver @timoni(update:semver:*)
			namespace: "test"
			values: {}
		}
		url: {
			module: url:     _url
			module: version: "1.0.0" @timoni(update:semver:*)
			namespace: "test"
			values: {}
		}
	}
}
`
	files := writeBundleFiles(t, map[string]string{"bundle.cue": bundle})
	updater := NewBundleUpdater(cuecontext.New(), files)
	g.Expect(updater.Load()).To(Succeed())

	plan, err := updater.Plan(context.Background(), &fakeVersionLister{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(plan.Changes).To(BeEmpty())
	g.Expect(skipFor(plan, "version").Reason).To(Equal("module version is set from a runtime value"))
	g.Expect(skipFor(plan, "url").Reason).To(Equal("module url is set from a runtime value"))
}

func TestBundleUpdater_CueModule(t *testing.T) {
	g := NewWithT(t)
	const (
		podinfo = "oci://registry.local/modules/podinfo"
		redis   = "oci://registry.local/modules/redis"
		nginx   = "oci://registry.local/modules/nginx"
	)
	lister := &fakeVersionLister{versions: map[string][]string{
		podinfo: versions("6.2.0", "6.1.0"),
		redis:   versions("8.1.0", "8.0.0"),
		nginx:   versions("2.0.0", "1.1.0", "1.0.0"),
	}}

	root := t.TempDir()
	dir := filepath.Join(root, "fleet")
	write := func(name, content string) string {
		t.Helper()
		path := filepath.Join(root, name)
		g.Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
		g.Expect(os.WriteFile(path, []byte(content), 0o644)).To(Succeed())
		return path
	}

	write("fleet/cue.mod/module.cue", `module: "example.com/fleet"
language: version: "v0.14.0"
`)
	// A package located outside the module, reachable through a symlink.
	write("outside/shared/shared.cue", `package shared

instances: shared: {
	module: url:     "`+nginx+`"
	module: version: "1.0.0" @timoni(update:semver:*)
	namespace: "test"
	values: {}
}
`)
	g.Expect(os.Symlink(filepath.Join(root, "outside", "shared"), filepath.Join(dir, "shared"))).To(Succeed())
	// A package imported transitively through the apps package.
	versionsFile := write("fleet/versions/versions.cue", `package versions

nginx: "1.0.0" @timoni(update:semver:1.x)
`)
	write("fleet/cue.mod/pkg/example.com/vendored/versions.cue", `package vendored

podinfo: "6.1.0"
`)
	write("fleet/apps/cluster.cue", `package apps

#cluster: name: string
`)
	redisFile := write("fleet/apps/redis.cue", `package apps

instances: redis: {
	module: url:     "`+redis+`"
	module: version: "8.0.0" @timoni(update:semver:8.x)
	namespace: #cluster.name
	values: {}
}
`)
	write("fleet/apps/podinfo.cue", `package apps

import "example.com/vendored"

instances: podinfo: {
	module: url:     "`+podinfo+`"
	module: version: vendored.podinfo
	namespace: "test"
	values: {}
}
`)
	write("fleet/apps/nginx.cue", `package apps

import "example.com/fleet/versions"

instances: nginx: {
	module: url:     "`+nginx+`"
	module: version: versions.nginx
	namespace: "test"
	values: {}
}
`)
	entry := write("fleet/apps/bundle.cue", `import (
	"example.com/fleet/apps"
	"example.com/fleet/shared"
)

_cluster: {
	name: *"default" | string @timoni(runtime:string:TIMONI_CLUSTER_NAME)
}

bundle: {
	apiVersion: "v1alpha1"
	name:       "apps"
	instances:  (apps & {#cluster: _cluster}).instances
	instances:  shared.instances
}
`)
	original, err := os.ReadFile(entry)
	g.Expect(err).ToNot(HaveOccurred())

	updater := NewBundleUpdater(cuecontext.New(), []string{entry})
	updater.SetWorkdir(dir)
	g.Expect(updater.SetLevel(UpdateLevelMajor)).To(Succeed())
	g.Expect(updater.Load()).To(Succeed())

	plan, err := updater.Plan(context.Background(), lister)
	g.Expect(err).ToNot(HaveOccurred())

	// The version literals in the imported package files are rewritten,
	// including the ones in packages imported transitively.
	change := changeFor(plan, "redis")
	g.Expect(change).ToNot(BeNil())
	g.Expect(change.ToVersion).To(Equal("8.1.0"))
	g.Expect(change.Files).To(HaveExactElements(redisFile))

	change = changeFor(plan, "nginx")
	g.Expect(change).ToNot(BeNil())
	g.Expect(change.ToVersion).To(Equal("1.1.0"))
	g.Expect(change.Files).To(HaveExactElements(versionsFile))

	// The version literals in the vendored dependency and in the
	// package located outside the module are left alone.
	g.Expect(skipFor(plan, "podinfo").Reason).To(Equal("module version is defined in versions.cue outside the bundle CUE module"))
	g.Expect(skipFor(plan, "shared").Reason).To(Equal("module version is defined in shared.cue outside the bundle CUE module"))
	g.Expect(plan.Changes).To(HaveLen(2))

	g.Expect(updater.Apply(plan)).To(Succeed())

	out, err := updater.Format(redisFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(out)).To(ContainSubstring(`version: "8.1.0" @timoni(update:semver:8.x)`))

	out, err = updater.Format(versionsFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(out)).To(ContainSubstring(`nginx: "1.1.0" @timoni(update:semver:1.x)`))

	out, err = updater.Format(entry)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(out).To(Equal(original))

	// The updated bundle evaluates with the rewritten package file.
	v := updater.Value().LookupPath(cuePath("bundle.instances.redis.module.version"))
	g.Expect(v).To(WithTransform(cueString, Equal("8.1.0")))
}

func TestBundleUpdater_Prerelease(t *testing.T) {
	const module = "oci://registry.local/modules/cert-manager"
	lister := &fakeVersionLister{versions: map[string][]string{
		module: versions("1.22.0-1", "1.21.2-0", "1.21.1-5", "1.21.1-4", "1.21.0", "1.20.0"),
	}}
	bundleFor := func(version string) string {
		return `
bundle: {
	apiVersion: "v1alpha1"
	name:       "test"
	instances: "cert-manager": {
		module: url:     "` + module + `"
		module: version: ` + version + `
		namespace: "test"
		values: {}
	}
}
`
	}

	tests := []struct {
		name    string
		version string
		level   string
		want    string
		skip    string
	}{
		{name: "pre-release tracks the constraint without the suffix", version: `"1.21.1-4" @timoni(update:semver:1.21.x)`, want: "1.21.2-0"},
		{name: "pre-release picks the newest pre-release", version: `"1.21.1-4" @timoni(update:semver:*)`, want: "1.22.0-1"},
		{name: "pre-release tracks the build number of an exact constraint", version: `"1.21.1-4" @timoni(update:semver:1.21.1)`, want: "1.21.1-5"},
		{name: "pre-release follows the patch level", version: `"1.21.1-4"`, level: UpdateLevelPatch, want: "1.21.2-0"},
		{name: "release ignores pre-releases", version: `"1.20.0" @timoni(update:semver:*)`, want: "1.21.0"},
		{name: "release is up to date when only pre-releases are newer", version: `"1.21.0" @timoni(update:semver:*)`, skip: "up to date"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			files := writeBundleFiles(t, map[string]string{"bundle.cue": bundleFor(tt.version)})
			updater := NewBundleUpdater(cuecontext.New(), files)
			if tt.level != "" {
				g.Expect(updater.SetLevel(tt.level)).To(Succeed())
			}
			g.Expect(updater.Load()).To(Succeed())

			plan, err := updater.Plan(context.Background(), lister)
			g.Expect(err).ToNot(HaveOccurred())
			if tt.skip != "" {
				g.Expect(plan.Changes).To(BeEmpty())
				g.Expect(skipFor(plan, "cert-manager").Reason).To(Equal(tt.skip))
				return
			}
			g.Expect(plan.Changes).To(HaveLen(1))
			g.Expect(plan.Changes[0].ToVersion).To(Equal(tt.want))
		})
	}
}
