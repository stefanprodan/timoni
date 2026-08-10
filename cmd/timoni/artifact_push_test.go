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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

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
	aSource := "org.opencontainers.image.source=https://host/repo.git"
	aRevision := "org.opencontainers.image.revision=1.0.0"

	// Push the artifact to registry
	output, err := executeCommand(fmt.Sprintf(
		"artifact push oci://%s -f %s -t %s -a '%s' -a '%s' -a '%s' --content-type=generic",
		aURL,
		aPath,
		aTag,
		aLicense,
		aRevision,
		aSource,
	))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(ContainSubstring(aURL))

	// List the artifacts
	output, err = executeCommand(fmt.Sprintf(
		"artifact list oci://%s",
		aURL,
	))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(ContainSubstring(aTag))

	// Pull the artifact from registry
	image, err := crane.Pull(fmt.Sprintf("%s:%s", aURL, aTag))
	g.Expect(err).ToNot(HaveOccurred())

	// Extract the manifest
	manifest, err := image.Manifest()
	g.Expect(err).ToNot(HaveOccurred())

	// Verify that annotations exist in manifest
	g.Expect(manifest.Annotations[apiv1.CreatedAnnotation]).ToNot(BeEmpty())
	g.Expect(manifest.Annotations[apiv1.SourceAnnotation]).To(BeEquivalentTo("https://host/repo.git"))
	g.Expect(manifest.Annotations[apiv1.RevisionAnnotation]).To(BeEquivalentTo(aTag))
	g.Expect(manifest.Annotations["org.opencontainers.image.licenses"]).To(BeEquivalentTo("Apache-2.0"))

	// Verify media types
	g.Expect(manifest.MediaType).To(Equal(types.OCIManifestSchema1))
	g.Expect(manifest.Config.MediaType).To(BeEquivalentTo(apiv1.ConfigMediaType))
	g.Expect(len(manifest.Layers)).To(BeEquivalentTo(1))
	g.Expect(manifest.Layers[0].MediaType).To(BeEquivalentTo(apiv1.ContentMediaType))
	g.Expect(manifest.Layers[0].Annotations[apiv1.ContentTypeAnnotation]).To(BeEquivalentTo("generic"))
}

func Test_PushArtifact_Symlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	g := NewWithT(t)

	// Create a dir with a relative symlink to a file living outside of it.
	tmpDir := t.TempDir()
	aPath := filepath.Join(tmpDir, "artifact")
	g.Expect(os.MkdirAll(aPath, 0o755)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(aPath, "main.cue"), []byte("main"), 0o644)).To(Succeed())
	sharedFile := filepath.Join(tmpDir, "shared", "extra.cue")
	g.Expect(os.MkdirAll(filepath.Dir(sharedFile), 0o755)).To(Succeed())
	g.Expect(os.WriteFile(sharedFile, []byte("extra"), 0o644)).To(Succeed())
	g.Expect(os.Symlink(filepath.Join("..", "shared", "extra.cue"),
		filepath.Join(aPath, "extra.cue"))).To(Succeed())

	aURL := fmt.Sprintf("%s/%s", dockerRegistry, rnd("my-artifact", 5))

	// By default the symlinked file is left out of the artifact.
	_, err := executeCommand(fmt.Sprintf("artifact push oci://%s -f %s -t skip --content-type=generic", aURL, aPath))
	g.Expect(err).ToNot(HaveOccurred())

	pullDir := filepath.Join(t.TempDir(), "skip")
	g.Expect(os.MkdirAll(pullDir, 0o755)).To(Succeed())
	_, err = executeCommand(fmt.Sprintf("artifact pull oci://%s:skip -o %s", aURL, pullDir))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(filepath.Join(pullDir, "main.cue")).To(BeARegularFile())
	g.Expect(filepath.Join(pullDir, "extra.cue")).ToNot(BeAnExistingFile())

	// With the opt-in, the symlinked file is materialized in the artifact.
	_, err = executeCommand(fmt.Sprintf("artifact push oci://%s -f %s -t resolve --content-type=generic --resolve-symlinks", aURL, aPath))
	g.Expect(err).ToNot(HaveOccurred())

	pullDir = filepath.Join(t.TempDir(), "resolve")
	g.Expect(os.MkdirAll(pullDir, 0o755)).To(Succeed())
	_, err = executeCommand(fmt.Sprintf("artifact pull oci://%s:resolve -o %s", aURL, pullDir))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(filepath.Join(pullDir, "main.cue")).To(BeARegularFile())
	g.Expect(filepath.Join(pullDir, "extra.cue")).To(BeARegularFile())
}

func TestPushArtifactRejectsInvalidTagsBeforeInput(t *testing.T) {
	g := NewWithT(t)

	_, err := executeCommand("artifact push oci://registry.example.com/org/artifact -f /does/not/exist -t 1.0.0 -t .invalid")
	g.Expect(err).To(MatchError(ContainSubstring("invalid OCI tag")))
}

func TestPushArtifactRejectsInvalidSignerBeforeInput(t *testing.T) {
	g := NewWithT(t)

	_, err := executeCommand("artifact push oci://registry.example.com/org/artifact -f /does/not/exist -t 1.0.0 --sign unsupported")
	g.Expect(err).To(MatchError(ContainSubstring("signer not supported: unsupported")))
}

func TestPushArtifactRejectsMissingCosignBeforeInput(t *testing.T) {
	t.Setenv("PATH", "")
	g := NewWithT(t)

	_, err := executeCommand("artifact push oci://registry.example.com/org/artifact -f /does/not/exist -t 1.0.0 --sign cosign")
	g.Expect(err).To(MatchError(ContainSubstring("executing cosign failed")))
}

func TestPushArtifactRejectsInvalidOutputBeforeInput(t *testing.T) {
	g := NewWithT(t)

	_, err := executeCommand("artifact push oci://registry.example.com/org/artifact -f /does/not/exist -t 1.0.0 --output invalid")
	g.Expect(err).To(MatchError("unknown --output=invalid, can be yaml or json"))
}

func Test_PushArtifact_OutputJSON(t *testing.T) {
	g := NewWithT(t)
	aPath := "testdata/module-values"
	aURL := fmt.Sprintf("%s/%s", dockerRegistry, rnd("my-artifact", 5))
	aTag := "1.0.0"

	output, err := executeCommand(fmt.Sprintf(
		"artifact push oci://%s -f %s -t %s --content-type=generic -o json",
		aURL,
		aPath,
		aTag,
	))
	g.Expect(err).ToNot(HaveOccurred())

	var info struct {
		URL        string `json:"url"`
		Repository string `json:"repository"`
		Tag        string `json:"tag"`
		Digest     string `json:"digest"`
	}
	g.Expect(json.Unmarshal([]byte(output), &info)).To(Succeed())

	image, err := crane.Pull(fmt.Sprintf("%s:%s", aURL, aTag))
	g.Expect(err).ToNot(HaveOccurred())
	digest, err := image.Digest()
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(info.Repository).To(Equal(aURL))
	g.Expect(info.Tag).To(Equal(aTag))
	g.Expect(info.Digest).To(Equal(digest.String()))
	g.Expect(info.URL).To(Equal(fmt.Sprintf("oci://%s@%s", aURL, digest.String())))
}
