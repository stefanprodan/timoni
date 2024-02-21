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

package engine

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func TestCopyModule_Ignore(t *testing.T) {
	g := NewWithT(t)
	moduleRoot := path.Join(t.TempDir(), "module")

	err := CopyModule("testdata/module", moduleRoot)
	g.Expect(err).ToNot(HaveOccurred())

	// Walk the original module and check that all files exist in tmp excluding ignored
	fsErr := filepath.Walk("testdata/module", func(path string, info fs.FileInfo, err error) error {
		if !info.IsDir() {
			tmpPath := filepath.Join(moduleRoot, strings.TrimPrefix(path, "testdata/module"))
			if _, err := os.Stat(tmpPath); err != nil && os.IsNotExist(err) && !strings.Contains(tmpPath, "ignore") {
				return fmt.Errorf("file '%s' should exist in tmp module", path)
			}
		}

		return nil
	})
	g.Expect(fsErr).ToNot(HaveOccurred())

	// Walk the tmp module and check ignored files
	fsErr = filepath.Walk(moduleRoot, func(path string, info fs.FileInfo, err error) error {
		if strings.Contains(info.Name(), "ignore") {
			return fmt.Errorf("file '%s' should not exist in tmp module", path)
		}
		return nil
	})
	g.Expect(fsErr).ToNot(HaveOccurred())
}

func TestIsOCIUrl(t *testing.T) {
	g := NewWithT(t)
	g.Expect(IsOCIUrl("oci://foo/bar")).To(BeTrueBecause("oci:// is an OCI URL"))
	g.Expect(IsOCIUrl("file://afile.txt")).To(BeFalseBecause("file:// is not an OCI URL"))
}

func TestIsFileUrl(t *testing.T) {
	g := NewWithT(t)
	g.Expect(IsFileUrl("file://afile.txt")).To(BeTrueBecause("file:// is a file URL"))
	g.Expect(IsFileUrl("oci://foo/bar")).To(BeFalseBecause("oci:// is not a file URL"))
}
