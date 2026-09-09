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
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/oci"
)

type localArtifactFallback struct{}

func (localArtifactFallback) ListVersions(_ context.Context, _ string) ([]string, error) {
	return []string{"1.0.0"}, nil
}

func (localArtifactFallback) ResolveDigest(_ context.Context, _, _ string) (string, error) {
	return "sha256:" + strings.Repeat("a", 64), nil
}

type remoteVersionFallback struct{}

func (remoteVersionFallback) ListVersions(_ context.Context, _ string) ([]string, error) {
	return []string{"1.2.0"}, nil
}

func (remoteVersionFallback) ResolveDigest(_ context.Context, _, _ string) (string, error) {
	return "sha256:" + strings.Repeat("b", 64), nil
}

func TestLocalModuleArtifactLister(t *testing.T) {
	g := NewWithT(t)
	root := t.TempDir()
	build, err := oci.BuildModuleImage("../oci/testdata/module", nil, map[string]string{
		apiv1.VersionAnnotation: "1.1.0",
	})
	g.Expect(err).ToNot(HaveOccurred())
	archive := filepath.Join(root, "module.oci.tar")
	g.Expect(oci.WriteImage(build.Image, archive, oci.FormatArchive, []string{"1.1.0"})).To(Succeed())
	g.Expect(build.Close()).To(Succeed())

	repository := "oci://registry.example/team/module"
	_, err = NewLocalModuleArtifactLister([]string{archive}, nil)
	g.Expect(err).To(MatchError(ContainSubstring("expected oci://repository=path")))
	_, err = NewLocalModuleArtifactLister([]string{"oci://=" + archive}, nil)
	g.Expect(err).To(MatchError(ContainSubstring("expected oci://repository=path")))

	lister, err := NewLocalModuleArtifactLister([]string{repository + "=" + archive}, localArtifactFallback{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(lister.ListVersions(context.Background(), repository)).To(Equal([]string{"1.1.0", "1.0.0"}))
	digest, err := lister.ResolveDigest(context.Background(), repository, "1.1.0")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(digest).To(HavePrefix("sha256:"))
	source, err := lister.ResolveSource(context.Background(), repository, "1.1.0")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(source).To(Equal("file://" + archive))
	identity, ok, err := lister.ResolveIdentity(source, root)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(identity).To(Equal(repository))

	remoteLister, err := NewLocalModuleArtifactLister([]string{repository + "=" + archive}, remoteVersionFallback{})
	g.Expect(err).ToNot(HaveOccurred())
	toRemote, err := resolveIndexedSource(context.Background(), remoteLister, &updateTarget{
		repository: repository,
		source:     source,
		indexed:    true,
		sourceLit:  &bundleLiteral{file: &bundleFile{origin: filepath.Join(root, "bundle.cue")}},
	}, "1.2.0")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(toRemote).To(Equal(repository))

	replacement, err := oci.BuildModuleImage("../oci/testdata/module", nil, map[string]string{
		apiv1.VersionAnnotation: "1.2.0",
	})
	g.Expect(err).ToNot(HaveOccurred())
	replacementArchive := filepath.Join(root, "replacement.oci.tar")
	g.Expect(oci.WriteImage(replacement.Image, replacementArchive, oci.FormatArchive, []string{"1.2.0"})).To(Succeed())
	g.Expect(replacement.Close()).To(Succeed())
	g.Expect(os.Rename(replacementArchive, archive)).To(Succeed())
	_, err = lister.ResolveDigest(context.Background(), repository, "1.1.0")
	g.Expect(err).To(MatchError(ContainSubstring("version mismatch")))
}
