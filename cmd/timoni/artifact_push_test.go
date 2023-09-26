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

package main

import (
	"fmt"
	"testing"

	"github.com/fluxcd/pkg/oci"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/v1/types"
	. "github.com/onsi/gomega"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func Test_PushArtifact(t *testing.T) {
	aPath := "testdata/module-values"

	g := NewWithT(t)
	aURL := fmt.Sprintf("%s/%s", dockerRegistry, rnd("my-artifact", 5))
	aTag := "1.0.0"
	aLicense := "org.opencontainers.image.licenses=Apache-2.0"
	aRevision := "org.opencontainers.image.revision=1.0.0"

	// Push the artifact to registry
	output, err := executeCommand(fmt.Sprintf(
		"artifact push oci://%s -f %s -t %s -a '%s' -a '%s'",
		aURL,
		aPath,
		aTag,
		aLicense,
		aRevision,
	))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(ContainSubstring(aURL))

	// Pull the artifact from registry
	image, err := crane.Pull(fmt.Sprintf("%s:%s", aURL, aTag))
	g.Expect(err).ToNot(HaveOccurred())

	// Extract the manifest
	manifest, err := image.Manifest()
	g.Expect(err).ToNot(HaveOccurred())

	// Verify that annotations exist in manifest
	g.Expect(manifest.Annotations[oci.CreatedAnnotation]).ToNot(BeEmpty())
	g.Expect(manifest.Annotations[oci.RevisionAnnotation]).To(BeEquivalentTo(aTag))
	g.Expect(manifest.Annotations["org.opencontainers.image.licenses"]).To(BeEquivalentTo("Apache-2.0"))

	// Verify media types
	g.Expect(manifest.MediaType).To(Equal(types.OCIManifestSchema1))
	g.Expect(manifest.Config.MediaType).To(BeEquivalentTo(apiv1.ConfigMediaType))
	g.Expect(len(manifest.Layers)).To(BeEquivalentTo(1))
	g.Expect(manifest.Layers[0].MediaType).To(BeEquivalentTo(apiv1.ContentMediaType))
	g.Expect(manifest.Layers[0].Annotations[apiv1.ContentTypeAnnotation]).To(BeEquivalentTo("generic"))
}
