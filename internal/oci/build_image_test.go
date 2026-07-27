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
	"testing"

	. "github.com/onsi/gomega"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestBuildImages(t *testing.T) {
	g := NewWithT(t)
	annotations := map[string]string{"example.com/key": "value"}

	artifact, err := BuildArtifactImage("testdata/module", []string{"timoni.ignore"}, "generic", annotations)
	g.Expect(err).ToNot(HaveOccurred())
	defer artifact.Close()

	manifest, err := artifact.Image.Manifest()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(manifest.Annotations).To(HaveKeyWithValue("example.com/key", "value"))
	g.Expect(manifest.Layers).To(HaveLen(1))
	g.Expect(manifest.Layers[0].Annotations).To(HaveKeyWithValue(apiv1.ContentTypeAnnotation, "generic"))
	digest, err := artifact.Image.Digest()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(artifact.Digest).To(Equal(digest))

	ignorePaths := make([]string, 1, 2)
	ignorePaths[0] = "timoni.ignore"
	backing := ignorePaths[:cap(ignorePaths)]
	backing[1] = "preserve"
	module, err := BuildModuleImage("testdata/module", ignorePaths, annotations)
	g.Expect(err).ToNot(HaveOccurred())
	defer module.Close()

	manifest, err = module.Image.Manifest()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(manifest.Layers).To(HaveLen(2))
	g.Expect(manifest.Layers[0].Annotations).To(HaveKeyWithValue(apiv1.ContentTypeAnnotation, apiv1.TimoniModVendorContentType))
	g.Expect(manifest.Layers[1].Annotations).To(HaveKeyWithValue(apiv1.ContentTypeAnnotation, apiv1.TimoniModContentType))
	g.Expect(backing[1]).To(Equal("preserve"))
}
