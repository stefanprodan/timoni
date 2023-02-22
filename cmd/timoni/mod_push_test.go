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
	"github.com/fluxcd/pkg/oci"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/v1/types"
	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"testing"

	. "github.com/onsi/gomega"
)

func Test_PushMod(t *testing.T) {
	modPath := "testdata/cs"

	g := NewWithT(t)
	modURL := fmt.Sprintf("%s/%s", dockerRegistry, rnd("my-mod", 5))
	modVer := "1.0.0"

	// Push the module to registry
	output, err := executeCommand(fmt.Sprintf(
		"mod push %s oci://%s -v %s",
		modPath,
		modURL,
		modVer,
	))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(ContainSubstring(modURL))

	// Pull the module's artifact from registry
	image, err := crane.Pull(fmt.Sprintf("%s:%s", modURL, modVer))
	g.Expect(err).ToNot(HaveOccurred())

	// Extract the manifest
	manifest, err := image.Manifest()
	g.Expect(err).ToNot(HaveOccurred())

	// Verify that annotations exist in manifest
	g.Expect(manifest.Annotations[oci.CreatedAnnotation]).ToNot(BeEmpty())
	g.Expect(manifest.Annotations[oci.RevisionAnnotation]).To(BeEquivalentTo(modVer))

	// Verify media types
	g.Expect(manifest.MediaType).To(Equal(types.OCIManifestSchema1))
	g.Expect(manifest.Config.MediaType).To(BeEquivalentTo(apiv1.ConfigMediaType))
	g.Expect(len(manifest.Layers)).To(BeEquivalentTo(1))
	g.Expect(manifest.Layers[0].MediaType).To(BeEquivalentTo(apiv1.ContentMediaType))
}
