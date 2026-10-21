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
	"os"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/gomega"
)

func Test_BuildArtifact(t *testing.T) {
	g := NewWithT(t)
	output := filepath.Join(t.TempDir(), "artifact.tar")

	result, err := executeCommand(fmt.Sprintf(
		"artifact build -f testdata/module -o %s -t 1.0.0 -t latest -a org.opencontainers.image.created=2024-01-02T03:04:05Z",
		output,
	))

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(BeAnExistingFile())
	g.Expect(result).To(ContainSubstring("digest: sha256:"))
}

func Test_BuildArtifactRequiresOutput(t *testing.T) {
	g := NewWithT(t)
	_, err := executeCommand("artifact build -f testdata/module")
	g.Expect(err).To(MatchError(ContainSubstring("output path is required")))
}

func Test_BuildArtifactValidatesFormatBeforeSource(t *testing.T) {
	g := NewWithT(t)
	_, err := executeCommand("artifact build -f missing -o output --format invalid")
	g.Expect(err).To(MatchError("unsupported OCI output format \"invalid\""))
}

func Test_BuildArtifactSymlinks(t *testing.T) {
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

	// By default the symlinked file is left out of the artifact.
	output := filepath.Join(t.TempDir(), "skip")
	_, err := executeCommand(fmt.Sprintf(
		"artifact build -f %s -o %s -t 1.0.0 --format oci-layout",
		aPath,
		output,
	))
	g.Expect(err).ToNot(HaveOccurred())
	files := layoutFiles(g, output, "generic")
	g.Expect(files).To(HaveKeyWithValue("main.cue", "main"))
	g.Expect(files).ToNot(HaveKey("extra.cue"))

	// With the opt-in, the symlinked file is materialized in the artifact.
	output = filepath.Join(t.TempDir(), "resolve")
	_, err = executeCommand(fmt.Sprintf(
		"artifact build -f %s -o %s -t 1.0.0 --format oci-layout --resolve-symlinks",
		aPath,
		output,
	))
	g.Expect(err).ToNot(HaveOccurred())
	files = layoutFiles(g, output, "generic")
	g.Expect(files).To(HaveKeyWithValue("main.cue", "main"))
	g.Expect(files).To(HaveKeyWithValue("extra.cue", "extra"))
}
