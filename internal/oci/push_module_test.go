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

	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/types"
	. "github.com/onsi/gomega"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestImageFromLocal(t *testing.T) {
	g := NewWithT(t)

	build, err := BuildModuleImage("testdata/module", []string{"timoni.ignore"}, map[string]string{
		apiv1.CreatedAnnotation: "2024-01-02T03:04:05Z",
		apiv1.VersionAnnotation: "1.0.0",
	})
	g.Expect(err).ToNot(HaveOccurred())
	defer build.Close()

	t.Run("archive file", func(t *testing.T) {
		g := NewWithT(t)
		dst := filepath.Join(t.TempDir(), "module.tar")
		g.Expect(WriteImage(build.Image, dst, FormatArchive, []string{"1.0.0"})).To(Succeed())

		image, cleanup, err := imageFromLocal(dst)
		g.Expect(err).ToNot(HaveOccurred())
		defer cleanup()

		digest, err := image.Digest()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(digest).To(Equal(build.Digest))
	})

	t.Run("rejects directory", func(t *testing.T) {
		g := NewWithT(t)
		dst := filepath.Join(t.TempDir(), "module")
		g.Expect(WriteImage(build.Image, dst, FormatLayout, []string{"1.0.0"})).To(Succeed())

		_, _, err := imageFromLocal(dst)
		g.Expect(err).To(MatchError(ContainSubstring("module not found at path")))
	})

	t.Run("missing source", func(t *testing.T) {
		g := NewWithT(t)
		_, _, err := imageFromLocal(filepath.Join(t.TempDir(), "missing.tar"))
		g.Expect(err).To(MatchError(ContainSubstring("module not found at path")))
	})

	t.Run("corrupt archive", func(t *testing.T) {
		g := NewWithT(t)
		dst := filepath.Join(t.TempDir(), "corrupt.tar")
		g.Expect(os.WriteFile(dst, []byte("not a tar archive"), 0o644)).To(Succeed())

		_, _, err := imageFromLocal(dst)
		g.Expect(err).To(MatchError(ContainSubstring("extracting OCI archive failed")))
	})
}

func TestModuleVersion(t *testing.T) {
	g := NewWithT(t)

	build, err := BuildModuleImage("testdata/module", []string{"timoni.ignore"}, map[string]string{
		apiv1.CreatedAnnotation: "2024-01-02T03:04:05Z",
		apiv1.VersionAnnotation: "1.0.0",
	})
	g.Expect(err).ToNot(HaveOccurred())
	defer build.Close()

	t.Run("reads version annotation", func(t *testing.T) {
		g := NewWithT(t)
		dst := filepath.Join(t.TempDir(), "module.tar")
		g.Expect(WriteImage(build.Image, dst, FormatArchive, []string{"1.0.0"})).To(Succeed())

		version, err := ModuleVersion(dst)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(version).To(Equal("1.0.0"))
	})

	t.Run("rejects missing annotation", func(t *testing.T) {
		g := NewWithT(t)
		// Build an image without the version annotation.
		noVersion, err := BuildModuleImage("testdata/module", []string{"timoni.ignore"}, map[string]string{
			apiv1.CreatedAnnotation: "2024-01-02T03:04:05Z",
		})
		g.Expect(err).ToNot(HaveOccurred())
		defer noVersion.Close()

		dst := filepath.Join(t.TempDir(), "module.tar")
		g.Expect(WriteImage(noVersion.Image, dst, FormatArchive, []string{"1.0.0"})).To(Succeed())

		_, err = ModuleVersion(dst)
		g.Expect(err).To(MatchError(ContainSubstring("module version annotation is missing")))
	})
}

func TestValidateModuleManifest(t *testing.T) {
	g := NewWithT(t)

	build, err := BuildModuleImage("testdata/module", []string{"timoni.ignore"}, map[string]string{
		apiv1.CreatedAnnotation: "2024-01-02T03:04:05Z",
		apiv1.VersionAnnotation: "1.0.0",
	})
	g.Expect(err).ToNot(HaveOccurred())
	defer build.Close()

	manifest, err := build.Image.Manifest()
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("valid module manifest", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(validateModuleManifest(manifest, "1.0.0")).To(Succeed())
	})

	t.Run("rejects non-OCI manifest media type", func(t *testing.T) {
		g := NewWithT(t)
		foreign := &gcrv1.Manifest{MediaType: types.DockerManifestSchema2}
		err := validateModuleManifest(foreign, "1.0.0")
		g.Expect(err).To(MatchError(ContainSubstring("unsupported manifest media type")))
	})

	t.Run("rejects foreign config media type", func(t *testing.T) {
		g := NewWithT(t)
		foreign := &gcrv1.Manifest{MediaType: types.OCIManifestSchema1}
		err := validateModuleManifest(foreign, "1.0.0")
		g.Expect(err).To(MatchError(ContainSubstring("unsupported config media type")))
	})

	t.Run("rejects missing version annotation", func(t *testing.T) {
		g := NewWithT(t)
		noVersion := &gcrv1.Manifest{
			MediaType:   types.OCIManifestSchema1,
			Config:      gcrv1.Descriptor{MediaType: apiv1.ConfigMediaType},
			Annotations: map[string]string{},
		}
		err := validateModuleManifest(noVersion, "1.0.0")
		g.Expect(err).To(MatchError(ContainSubstring("module version annotation is missing")))
	})

	t.Run("rejects version mismatch", func(t *testing.T) {
		g := NewWithT(t)
		err := validateModuleManifest(manifest, "2.0.0")
		g.Expect(err).To(MatchError(ContainSubstring("version mismatch")))
	})

	t.Run("rejects wrong layer count", func(t *testing.T) {
		g := NewWithT(t)
		oneLayer := &gcrv1.Manifest{
			MediaType: types.OCIManifestSchema1,
			Config:    gcrv1.Descriptor{MediaType: apiv1.ConfigMediaType},
			Annotations: map[string]string{
				apiv1.VersionAnnotation: "1.0.0",
			},
			Layers: []gcrv1.Descriptor{{MediaType: apiv1.ContentMediaType}},
		}
		err := validateModuleManifest(oneLayer, "1.0.0")
		g.Expect(err).To(MatchError(ContainSubstring("expected 2 layers")))
	})

	t.Run("rejects wrong layer content type", func(t *testing.T) {
		g := NewWithT(t)
		wrongLayers := &gcrv1.Manifest{
			MediaType: types.OCIManifestSchema1,
			Config:    gcrv1.Descriptor{MediaType: apiv1.ConfigMediaType},
			Annotations: map[string]string{
				apiv1.VersionAnnotation: "1.0.0",
			},
			Layers: []gcrv1.Descriptor{
				{
					MediaType: apiv1.ContentMediaType,
					Annotations: map[string]string{
						apiv1.ContentTypeAnnotation: "generic",
					},
				},
				{
					MediaType: apiv1.ContentMediaType,
					Annotations: map[string]string{
						apiv1.ContentTypeAnnotation: apiv1.TimoniModContentType,
					},
				},
			},
		}
		err := validateModuleManifest(wrongLayers, "1.0.0")
		g.Expect(err).To(MatchError(ContainSubstring("invalid module layer 0 content type")))
	})

	t.Run("rejects generic artifact", func(t *testing.T) {
		g := NewWithT(t)
		generic, err := BuildArtifactImage("testdata/module", []string{"timoni.ignore"}, "generic", map[string]string{
			apiv1.VersionAnnotation: "1.0.0",
		})
		g.Expect(err).ToNot(HaveOccurred())
		defer generic.Close()

		manifest, err := generic.Image.Manifest()
		g.Expect(err).ToNot(HaveOccurred())
		err = validateModuleManifest(manifest, "1.0.0")
		g.Expect(err).To(MatchError(ContainSubstring("expected 2 layers")))
	})
}

func TestLoadFromLocalRejectsForeignImage(t *testing.T) {
	g := NewWithT(t)

	// empty.Image has the Docker manifest media type and must be rejected
	// before any registry interaction.
	dst := filepath.Join(t.TempDir(), "foreign.tar")
	g.Expect(WriteImage(empty.Image, dst, FormatArchive, []string{"1.0.0"})).To(Succeed())

	image, cleanup, err := imageFromLocal(dst)
	g.Expect(err).ToNot(HaveOccurred())
	defer cleanup()

	manifest, err := image.Manifest()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(validateModuleManifest(manifest, "1.0.0")).To(MatchError(ContainSubstring("unsupported manifest media type")))
}
