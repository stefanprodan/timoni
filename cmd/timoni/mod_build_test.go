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
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	. "github.com/onsi/gomega"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/fscopy"
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
	g.Expect(manifest.Annotations).To(HaveKeyWithValue(apiv1.ImagesAnnotation,
		"cgr.dev/chainguard/timoni:latest-dev@sha256:b49fbaac0eedc22c1cfcd26684707179cccbed0df205171bae3e1bae61326a10"))
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

func Test_BuildModRejectsBuildMetadataVersion(t *testing.T) {
	g := NewWithT(t)
	output := filepath.Join(t.TempDir(), "module.tar")
	_, err := executeCommand(fmt.Sprintf(
		"mod build testdata/module -v 1.0.0+demo -o %s",
		output,
	))
	g.Expect(err).To(MatchError(ContainSubstring("version build metadata is not supported")))
}

func Test_BuildModSymlinks(t *testing.T) {
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

	// By default the symlinked file is left out of the artifact.
	output := filepath.Join(t.TempDir(), "skip")
	_, err := executeCommand(fmt.Sprintf(
		"mod build %s -v 1.0.0 -o %s --format oci-layout",
		modPath,
		output,
	))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(layoutFiles(g, output, apiv1.TimoniModContentType)).ToNot(HaveKey("extra.txt"))

	// With the opt-in, the symlinked file is materialized in the artifact.
	output = filepath.Join(t.TempDir(), "resolve")
	_, err = executeCommand(fmt.Sprintf(
		"mod build %s -v 1.0.0 -o %s --format oci-layout --resolve-symlinks",
		modPath,
		output,
	))
	g.Expect(err).ToNot(HaveOccurred())
	files := layoutFiles(g, output, apiv1.TimoniModContentType)
	g.Expect(files).To(HaveKeyWithValue("extra.txt", "extra"))
	g.Expect(files).To(HaveKey("values.cue"))
}

// layoutFiles returns the regular files packaged in the layer with the given
// content type of the single image in the OCI layout at the given path,
// keyed by their archive path.
func layoutFiles(g *WithT, path, contentType string) map[string]string {
	p, err := layout.FromPath(path)
	g.Expect(err).ToNot(HaveOccurred())
	index, err := p.ImageIndex()
	g.Expect(err).ToNot(HaveOccurred())
	indexManifest, err := index.IndexManifest()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(indexManifest.Manifests).To(HaveLen(1))
	image, err := index.Image(indexManifest.Manifests[0].Digest)
	g.Expect(err).ToNot(HaveOccurred())
	manifest, err := image.Manifest()
	g.Expect(err).ToNot(HaveOccurred())
	var layer v1.Layer
	for _, desc := range manifest.Layers {
		if desc.Annotations[apiv1.ContentTypeAnnotation] == contentType {
			layer, err = image.LayerByDigest(desc.Digest)
			g.Expect(err).ToNot(HaveOccurred())
		}
	}
	g.Expect(layer).ToNot(BeNil())

	rc, err := layer.Uncompressed()
	g.Expect(err).ToNot(HaveOccurred())
	defer rc.Close()

	files := map[string]string{}
	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		g.Expect(err).ToNot(HaveOccurred())
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(tr)
		g.Expect(err).ToNot(HaveOccurred())
		files[hdr.Name] = string(data)
	}
	return files
}
