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

package main

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/layout"
	. "github.com/onsi/gomega"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func Test_BuildMod(t *testing.T) {
	g := NewWithT(t)
	output := filepath.Join(t.TempDir(), "module.tar")

	result, err := executeCommand(fmt.Sprintf(
		"mod build testdata/module -v 1.0.0 -o %s -a org.opencontainers.image.created=2024-01-02T03:04:05Z",
		output,
	))

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(BeAnExistingFile())
	g.Expect(result).To(ContainSubstring("digest: sha256:"))
}

func Test_BuildModVersionOverridesAnnotation(t *testing.T) {
	g := NewWithT(t)
	output := filepath.Join(t.TempDir(), "module")

	_, err := executeCommand(fmt.Sprintf(
		"mod build testdata/module -v 1.0.0 -o %s --format oci-layout -a %s=9.9.9",
		output,
		apiv1.VersionAnnotation,
	))
	g.Expect(err).ToNot(HaveOccurred())

	path, err := layout.FromPath(output)
	g.Expect(err).ToNot(HaveOccurred())
	index, err := path.ImageIndex()
	g.Expect(err).ToNot(HaveOccurred())
	indexManifest, err := index.IndexManifest()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(indexManifest.Manifests).To(HaveLen(1))
	image, err := index.Image(indexManifest.Manifests[0].Digest)
	g.Expect(err).ToNot(HaveOccurred())
	manifest, err := image.Manifest()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(manifest.Annotations).To(HaveKeyWithValue(apiv1.VersionAnnotation, "1.0.0"))
}

func Test_BuildModRequiresVersion(t *testing.T) {
	g := NewWithT(t)
	output := filepath.Join(t.TempDir(), "module.tar")
	_, err := executeCommand(fmt.Sprintf("mod build testdata/module -o %s", output))
	g.Expect(err).To(MatchError(ContainSubstring("version is required")))
}

func Test_BuildModValidatesFormatBeforeSource(t *testing.T) {
	g := NewWithT(t)
	_, err := executeCommand("mod build missing -v 1.0.0 -o output --format invalid")
	g.Expect(err).To(MatchError("unsupported OCI output format \"invalid\""))
}
