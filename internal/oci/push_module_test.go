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
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/empty"
	. "github.com/onsi/gomega"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestLoadLocalImage(t *testing.T) {
	g := NewWithT(t)

	build, err := BuildModuleImage("testdata/module", []string{"timoni.ignore"}, map[string]string{
		apiv1.CreatedAnnotation: "2024-01-02T03:04:05Z",
		apiv1.VersionAnnotation: "1.0.0",
	})
	g.Expect(err).ToNot(HaveOccurred())
	defer build.Close()

	t.Run("layout directory", func(t *testing.T) {
		g := NewWithT(t)
		dst := filepath.Join(t.TempDir(), "module")
		g.Expect(WriteImage(build.Image, dst, FormatLayout, []string{"1.0.0"})).To(Succeed())

		image, digest, cleanup, err := loadLocalImage(dst, "1.0.0")
		g.Expect(err).ToNot(HaveOccurred())
		defer cleanup()
		g.Expect(digest).To(Equal(build.Digest))
		g.Expect(image).ToNot(BeNil())
	})

	t.Run("archive file", func(t *testing.T) {
		g := NewWithT(t)
		dst := filepath.Join(t.TempDir(), "module.tar")
		g.Expect(WriteImage(build.Image, dst, FormatArchive, []string{"1.0.0"})).To(Succeed())

		image, digest, cleanup, err := loadLocalImage(dst, "1.0.0")
		g.Expect(err).ToNot(HaveOccurred())
		defer cleanup()
		g.Expect(digest).To(Equal(build.Digest))
		g.Expect(image).ToNot(BeNil())
	})

	t.Run("falls back to first descriptor", func(t *testing.T) {
		g := NewWithT(t)
		dst := filepath.Join(t.TempDir(), "module")
		// The layout descriptor reference differs from the requested version,
		// so the loader must fall back to the first image.
		g.Expect(WriteImage(build.Image, dst, FormatLayout, []string{"9.9.9"})).To(Succeed())

		_, digest, cleanup, err := loadLocalImage(dst, "1.0.0")
		g.Expect(err).ToNot(HaveOccurred())
		defer cleanup()
		g.Expect(digest).To(Equal(build.Digest))
	})

	t.Run("missing source", func(t *testing.T) {
		g := NewWithT(t)
		_, _, _, err := loadLocalImage(filepath.Join(t.TempDir(), "missing.tar"), "1.0.0")
		g.Expect(err).To(MatchError(ContainSubstring("module not found at path")))
	})

	t.Run("rejects foreign media type", func(t *testing.T) {
		g := NewWithT(t)
		dst := filepath.Join(t.TempDir(), "foreign.tar")
		g.Expect(WriteImage(empty.Image, dst, FormatArchive, []string{"1.0.0"})).To(Succeed())

		_, _, _, err := loadLocalImage(dst, "1.0.0")
		g.Expect(err).To(MatchError(ContainSubstring("unsupported artifact type")))
	})

	t.Run("rejects version mismatch", func(t *testing.T) {
		g := NewWithT(t)
		dst := filepath.Join(t.TempDir(), "module.tar")
		g.Expect(WriteImage(build.Image, dst, FormatArchive, []string{"1.0.0"})).To(Succeed())

		_, _, _, err := loadLocalImage(dst, "2.0.0")
		g.Expect(err).To(MatchError(ContainSubstring("version mismatch")))
	})

	t.Run("rejects corrupt archive", func(t *testing.T) {
		g := NewWithT(t)
		dst := filepath.Join(t.TempDir(), "corrupt.tar")
		g.Expect(os.WriteFile(dst, []byte("not a tar archive"), 0o644)).To(Succeed())

		_, _, _, err := loadLocalImage(dst, "1.0.0")
		g.Expect(err).To(MatchError(ContainSubstring("extracting OCI archive failed")))
	})
}

func TestIsOCILayout(t *testing.T) {
	g := NewWithT(t)

	t.Run("recognizes layout directory", func(t *testing.T) {
		build, err := BuildModuleImage("testdata/module", nil, map[string]string{
			apiv1.VersionAnnotation: "1.0.0",
		})
		g.Expect(err).ToNot(HaveOccurred())
		defer build.Close()

		dst := filepath.Join(t.TempDir(), "module")
		g.Expect(WriteImage(build.Image, dst, FormatLayout, []string{"1.0.0"})).To(Succeed())
		g.Expect(IsOCILayout(dst)).To(BeTrue())
	})

	t.Run("rejects module source directory", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(IsOCILayout("testdata/module")).To(BeFalse())
	})

	t.Run("rejects missing path", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(IsOCILayout(filepath.Join(t.TempDir(), "missing"))).To(BeFalse())
	})

	t.Run("rejects archive file", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(IsOCILayout(filepath.Join(t.TempDir(), "module.tar"))).To(BeFalse())
	})
}
