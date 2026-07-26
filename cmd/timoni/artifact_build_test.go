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
