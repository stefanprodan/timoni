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
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/layout"
	. "github.com/onsi/gomega"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestWriteImage(t *testing.T) {
	g := NewWithT(t)
	build, err := BuildArtifactImage("testdata/module", []string{"timoni.ignore"}, "generic", map[string]string{
		apiv1.CreatedAnnotation: "2024-01-02T03:04:05Z",
	})
	g.Expect(err).ToNot(HaveOccurred())
	defer build.Close()

	t.Run("layout with optional references", func(t *testing.T) {
		g := NewWithT(t)
		dst := filepath.Join(t.TempDir(), "nested", "artifact")
		g.Expect(WriteImage(build.Image, dst, FormatLayout, []string{"1.0.0", "latest"})).To(Succeed())

		path, err := layout.FromPath(dst)
		g.Expect(err).ToNot(HaveOccurred())
		index, err := path.ImageIndex()
		g.Expect(err).ToNot(HaveOccurred())
		manifest, err := index.IndexManifest()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(manifest.Manifests).To(HaveLen(2))
		for i, tag := range []string{"1.0.0", "latest"} {
			g.Expect(manifest.Manifests[i].Digest).To(Equal(build.Digest))
			g.Expect(manifest.Manifests[i].Annotations).To(HaveKeyWithValue("org.opencontainers.image.ref.name", tag))
		}
	})

	t.Run("untagged deterministic archive", func(t *testing.T) {
		g := NewWithT(t)
		root := t.TempDir()
		first := filepath.Join(root, "first.tar")
		second := filepath.Join(root, "second.tar")
		g.Expect(WriteImage(build.Image, first, FormatArchive, nil)).To(Succeed())
		g.Expect(WriteImage(build.Image, second, FormatArchive, nil)).To(Succeed())

		firstBytes, err := os.ReadFile(first)
		g.Expect(err).ToNot(HaveOccurred())
		secondBytes, err := os.ReadFile(second)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(bytes.Equal(firstBytes, secondBytes)).To(BeTrue())
	})

	t.Run("existing archive remains untouched", func(t *testing.T) {
		g := NewWithT(t)
		dst := filepath.Join(t.TempDir(), "artifact.tar")
		marker := []byte("existing archive")
		g.Expect(os.WriteFile(dst, marker, 0o644)).To(Succeed())

		err := WriteImage(build.Image, dst, FormatArchive, nil)
		g.Expect(errors.Is(err, fs.ErrExist)).To(BeTrue())
		g.Expect(os.ReadFile(dst)).To(Equal(marker))
	})

	t.Run("existing layout remains untouched", func(t *testing.T) {
		g := NewWithT(t)
		dst := filepath.Join(t.TempDir(), "artifact")
		g.Expect(os.Mkdir(dst, 0o755)).To(Succeed())
		markerPath := filepath.Join(dst, "marker")
		marker := []byte("existing layout")
		g.Expect(os.WriteFile(markerPath, marker, 0o644)).To(Succeed())

		err := WriteImage(build.Image, dst, FormatLayout, nil)
		g.Expect(errors.Is(err, fs.ErrExist)).To(BeTrue())
		g.Expect(os.ReadFile(markerPath)).To(Equal(marker))
	})

	t.Run("invalid references leave no output", func(t *testing.T) {
		for _, format := range []LocalFormat{FormatArchive, FormatLayout} {
			t.Run(string(format), func(t *testing.T) {
				g := NewWithT(t)
				dst := filepath.Join(t.TempDir(), "artifact")

				err := WriteImage(build.Image, dst, format, []string{"valid", ""})
				g.Expect(err).To(MatchError(ContainSubstring("OCI reference cannot be empty")))
				_, statErr := os.Lstat(dst)
				g.Expect(os.IsNotExist(statErr)).To(BeTrue())
			})
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		g := NewWithT(t)
		err := WriteImage(build.Image, filepath.Join(t.TempDir(), "artifact"), LocalFormat("invalid"), nil)
		g.Expect(err).To(MatchError(ContainSubstring("unsupported OCI output format")))
	})
}
