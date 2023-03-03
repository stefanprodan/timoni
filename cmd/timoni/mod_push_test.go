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
	"github.com/stefanprodan/timoni/internal/engine"
)

func Test_PushMod(t *testing.T) {
	modPath := "testdata/module"

	g := NewWithT(t)
	modURL := fmt.Sprintf("%s/%s", dockerRegistry, rnd("my-mod", 5))
	modVer := "1.0.0"
	modLicense := "org.opencontainers.image.licenses=Apache-2.0"
	modAbout := "org.opencontainers.image.description=My, test."

	// Push the module to registry
	output, err := executeCommand(fmt.Sprintf(
		"mod push %s oci://%s -v %s -a '%s' -a '%s'",
		modPath,
		modURL,
		modVer,
		modLicense,
		modAbout,
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
	g.Expect(manifest.Annotations["org.opencontainers.image.licenses"]).To(BeEquivalentTo("Apache-2.0"))
	g.Expect(manifest.Annotations["org.opencontainers.image.description"]).To(BeEquivalentTo("My, test."))

	// Verify media types
	g.Expect(manifest.MediaType).To(Equal(types.OCIManifestSchema1))
	g.Expect(manifest.Config.MediaType).To(BeEquivalentTo(apiv1.ConfigMediaType))
	g.Expect(len(manifest.Layers)).To(BeEquivalentTo(1))
	g.Expect(manifest.Layers[0].MediaType).To(BeEquivalentTo(apiv1.ContentMediaType))

	// Push latest
	newVer := "1.0.1"
	_, err = executeCommand(fmt.Sprintf(
		"mod push %s oci://%s -v %s --latest",
		modPath,
		modURL,
		newVer,
	))
	g.Expect(err).ToNot(HaveOccurred())

	// Verify latest version
	image, err = crane.Pull(fmt.Sprintf("%s:%s", modURL, engine.LatestTag))
	g.Expect(err).ToNot(HaveOccurred())
	manifest, err = image.Manifest()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(manifest.Annotations[oci.RevisionAnnotation]).To(BeEquivalentTo(newVer))
}
