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
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/v1/types"
	. "github.com/onsi/gomega"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/fscopy"
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

	// Pull the module's artifact from registry
	image, err := crane.Pull(fmt.Sprintf("%s:%s", modURL, modVer))
	g.Expect(err).ToNot(HaveOccurred())

	digest, err := image.Digest()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(ContainSubstring(digest.String()))

	// Extract the manifest
	manifest, err := image.Manifest()
	g.Expect(err).ToNot(HaveOccurred())

	// Verify that annotations exist in manifest
	g.Expect(manifest.Annotations[apiv1.CreatedAnnotation]).ToNot(BeEmpty())
	g.Expect(manifest.Annotations[apiv1.RevisionAnnotation]).ToNot(BeEmpty())
	g.Expect(manifest.Annotations[apiv1.SourceAnnotation]).To(ContainSubstring("github.com"))
	g.Expect(manifest.Annotations[apiv1.VersionAnnotation]).To(BeEquivalentTo(modVer))
	g.Expect(manifest.Annotations["org.opencontainers.image.licenses"]).To(BeEquivalentTo("Apache-2.0"))
	g.Expect(manifest.Annotations["org.opencontainers.image.description"]).To(BeEquivalentTo("My, test."))

	// Verify media types
	g.Expect(manifest.MediaType).To(Equal(types.OCIManifestSchema1))
	g.Expect(manifest.Config.MediaType).To(BeEquivalentTo(apiv1.ConfigMediaType))
	g.Expect(len(manifest.Layers)).To(BeEquivalentTo(2))
	g.Expect(manifest.Layers[0].MediaType).To(BeEquivalentTo(apiv1.ContentMediaType))
	g.Expect(manifest.Layers[0].Annotations[apiv1.ContentTypeAnnotation]).To(BeEquivalentTo(apiv1.TimoniModVendorContentType))
	g.Expect(manifest.Layers[1].MediaType).To(BeEquivalentTo(apiv1.ContentMediaType))
	g.Expect(manifest.Layers[1].Annotations[apiv1.ContentTypeAnnotation]).To(BeEquivalentTo(apiv1.TimoniModContentType))

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
	image, err = crane.Pull(fmt.Sprintf("%s:%s", modURL, apiv1.LatestVersion))
	g.Expect(err).ToNot(HaveOccurred())
	manifest, err = image.Manifest()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(manifest.Annotations[apiv1.VersionAnnotation]).To(BeEquivalentTo(newVer))
}

func Test_PushMod_Symlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	g := NewWithT(t)

	// Copy the test module and add a relative symlink
	// to a file living outside the module root.
	tmpDir := t.TempDir()
	modPath := filepath.Join(tmpDir, "module")
	g.Expect(fscopy.CopyDir("testdata/module", modPath, fscopy.Options{})).To(Succeed())
	sharedFile := filepath.Join(tmpDir, "shared", "extra.txt")
	g.Expect(os.MkdirAll(filepath.Dir(sharedFile), 0o755)).To(Succeed())
	g.Expect(os.WriteFile(sharedFile, []byte("extra"), 0o644)).To(Succeed())
	g.Expect(os.Symlink(filepath.Join("..", "shared", "extra.txt"),
		filepath.Join(modPath, "extra.txt"))).To(Succeed())

	modURL := fmt.Sprintf("%s/%s", dockerRegistry, rnd("my-mod", 5))
	modVer := "1.0.0"

	// By default the symlinked file is materialized in the artifact.
	_, err := executeCommand(fmt.Sprintf("mod push %s oci://%s -v %s", modPath, modURL, modVer))
	g.Expect(err).ToNot(HaveOccurred())

	pullDir := filepath.Join(t.TempDir(), "follow")
	g.Expect(os.MkdirAll(pullDir, 0o755)).To(Succeed())
	_, err = executeCommand(fmt.Sprintf("mod pull oci://%s -v %s -o %s", modURL, modVer, pullDir))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(filepath.Join(pullDir, "extra.txt")).To(BeARegularFile())

	// With the opt-out, the symlinked file is left out of the artifact.
	t.Setenv("TIMONI_FOLLOW_SYMLINKS", "false")
	modVer = "1.0.1"
	_, err = executeCommand(fmt.Sprintf("mod push %s oci://%s -v %s", modPath, modURL, modVer))
	g.Expect(err).ToNot(HaveOccurred())

	pullDir = filepath.Join(t.TempDir(), "skip")
	g.Expect(os.MkdirAll(pullDir, 0o755)).To(Succeed())
	_, err = executeCommand(fmt.Sprintf("mod pull oci://%s -v %s -o %s", modURL, modVer, pullDir))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(filepath.Join(pullDir, "extra.txt")).ToNot(BeAnExistingFile())
}

func Test_PushModRejectsBuildMetadataVersion(t *testing.T) {
	g := NewWithT(t)
	modPath := "testdata/module"
	modURL := fmt.Sprintf("%s/%s", dockerRegistry, rnd("my-mod", 5))

	_, err := executeCommand(fmt.Sprintf(
		"mod push %s oci://%s -v 1.0.0+demo",
		modPath,
		modURL,
	))
	g.Expect(err).To(MatchError(ContainSubstring("version build metadata is not supported")))
}

func TestPushModRejectsInvalidSignerBeforeInput(t *testing.T) {
	g := NewWithT(t)

	_, err := executeCommand("mod push /does/not/exist oci://registry.example.com/org/module -v 1.0.0 --sign unsupported")
	g.Expect(err).To(MatchError(ContainSubstring("signer not supported: unsupported")))
}

func TestPushModRejectsMissingCosignBeforeInput(t *testing.T) {
	t.Setenv("PATH", "")
	g := NewWithT(t)

	_, err := executeCommand("mod push /does/not/exist oci://registry.example.com/org/module -v 1.0.0 --sign cosign")
	g.Expect(err).To(MatchError(ContainSubstring("executing cosign failed")))
}

func TestPushModRejectsInvalidOutputBeforeInput(t *testing.T) {
	g := NewWithT(t)

	_, err := executeCommand("mod push /does/not/exist oci://registry.example.com/org/module -v 1.0.0 --output invalid")
	g.Expect(err).To(MatchError("unknown --output=invalid, can be yaml or json"))
}
