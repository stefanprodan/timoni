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

package fetcher

import (
	"context"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/oci"
)

func TestLocalArtifactFetch(t *testing.T) {
	g := NewWithT(t)
	build, err := oci.BuildModuleImage("../../oci/testdata/module", nil, map[string]string{
		apiv1.VersionAnnotation: "1.1.0",
	})
	g.Expect(err).ToNot(HaveOccurred())
	archive := filepath.Join(t.TempDir(), "module.oci.tar")
	g.Expect(oci.WriteImage(build.Image, archive, oci.FormatArchive, []string{"1.1.0"})).To(Succeed())
	layout := filepath.Join(t.TempDir(), "module-layout")
	g.Expect(oci.WriteImage(build.Image, layout, oci.FormatLayout, []string{"1.1.0"})).To(Succeed())
	digest := build.Digest.String()
	g.Expect(build.Close()).To(Succeed())

	for _, source := range []string{archive, layout} {
		t.Run(filepath.Base(source), func(t *testing.T) {
			g := NewWithT(t)
			f, err := New(context.Background(), Options{
				Source:      "file://" + source,
				Version:     "1.1.0",
				Digest:      digest,
				Destination: t.TempDir(),
			})
			g.Expect(err).ToNot(HaveOccurred())
			mod, err := f.Fetch()
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(mod.Repository).To(Equal("file://" + source))
			g.Expect(mod.Version).To(Equal("1.1.0"))
			g.Expect(mod.Digest).To(Equal(digest))
			g.Expect(filepath.Join(f.GetModuleRoot(), "cue.mod", "module.cue")).To(BeAnExistingFile())
		})
	}
}
