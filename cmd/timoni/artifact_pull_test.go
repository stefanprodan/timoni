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
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func Test_PullArtifact(t *testing.T) {
	aPath := "testdata/crd/golden/cue.mod/gen/"

	g := NewWithT(t)
	aURL := fmt.Sprintf("%s/%s", dockerRegistry, rnd("my-crds", 5))
	aTag := "latest"

	// Push the artifact to registry
	output, err := executeCommand(fmt.Sprintf(
		"artifact push oci://%s -f %s -t %s --content-type=crds",
		aURL,
		aPath,
		aTag,
	))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(ContainSubstring(aURL))

	// Pull the artifact from registry
	tmpDir := t.TempDir()
	_, err = executeCommand(fmt.Sprintf(
		"artifact pull oci://%s -o %s --content-type=crds",
		aURL,
		tmpDir,
	))
	g.Expect(err).ToNot(HaveOccurred())

	// Walk the original module and check that all files exist in the pulled module
	fsErr := filepath.Walk(aPath, func(path string, info fs.FileInfo, err error) error {
		if !info.IsDir() {
			tmpPath := filepath.Join(tmpDir, strings.TrimPrefix(path, aPath))
			if _, err := os.Stat(tmpPath); err != nil && os.IsNotExist(err) {
				return fmt.Errorf("file '%s' should exist in pulled artifact", path)
			}
		}

		return nil
	})
	g.Expect(fsErr).ToNot(HaveOccurred())

	// Fail to pull on content mismatch
	_, err = executeCommand(fmt.Sprintf(
		"artifact pull oci://%s -o %s --content-type=test",
		aURL,
		tmpDir,
	))
	g.Expect(err).To(HaveOccurred())

}
