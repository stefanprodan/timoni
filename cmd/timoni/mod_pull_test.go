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

func Test_PullMod(t *testing.T) {
	g := NewWithT(t)
	modPath := "testdata/cs"
	modURL := fmt.Sprintf("%s/%s", dockerRegistry, rnd("my-mod", 5))
	modVer := "1.0.0"

	// Package the module as an OCI artifact and push it to registry
	_, err := executeCommand(fmt.Sprintf(
		"mod push %s oci://%s -v %s",
		modPath,
		modURL,
		modVer,
	))
	g.Expect(err).ToNot(HaveOccurred())

	// Pull the OCI artifact from registry and extract the module to tmp
	tmpDir := t.TempDir()
	_, err = executeCommand(fmt.Sprintf(
		"mod pull oci://%s -v %s -o %s",
		modURL,
		modVer,
		tmpDir,
	))
	g.Expect(err).ToNot(HaveOccurred())

	// Walk the original module and check that all files exist in the pulled module
	fsErr := filepath.Walk(modPath, func(path string, info fs.FileInfo, err error) error {
		if !info.IsDir() {
			tmpPath := filepath.Join(tmpDir, strings.TrimPrefix(path, modPath))
			if _, err := os.Stat(tmpPath); err != nil && os.IsNotExist(err) {
				return fmt.Errorf("file '%s' should exist in pulled module", path)
			}
		}

		return nil
	})
	g.Expect(fsErr).ToNot(HaveOccurred())
}
