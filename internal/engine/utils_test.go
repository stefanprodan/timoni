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
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/gomega"
)

func TestCopyDir_Ignore(t *testing.T) {
	g := NewWithT(t)
	moduleRoot := path.Join(t.TempDir(), "module")

	err := CopyDir("testdata/module", moduleRoot, true)
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

func TestCopyDir_Symlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	makeModule := func(t *testing.T) string {
		t.Helper()
		g := NewWithT(t)
		tmpDir := t.TempDir()
		srcDir := filepath.Join(tmpDir, "module")
		g.Expect(os.MkdirAll(srcDir, 0o755)).To(Succeed())
		// A shared file outside the module root, linked relatively.
		sharedFile := filepath.Join(tmpDir, "shared", "schema.cue")
		g.Expect(os.MkdirAll(filepath.Dir(sharedFile), 0o755)).To(Succeed())
		g.Expect(os.WriteFile(sharedFile, []byte("shared"), 0o644)).To(Succeed())
		g.Expect(os.Symlink(filepath.Join("..", "shared", "schema.cue"),
			filepath.Join(srcDir, "schema.cue"))).To(Succeed())
		return srcDir
	}

	t.Run("resolves symlinks on opt-in", func(t *testing.T) {
		g := NewWithT(t)
		srcDir := makeModule(t)
		dstDir := filepath.Join(t.TempDir(), "module")

		g.Expect(CopyDir(srcDir, dstDir, true)).To(Succeed())

		g.Expect(filepath.Join(dstDir, "schema.cue")).To(BeARegularFile())
	})

	t.Run("skips symlinks by default", func(t *testing.T) {
		g := NewWithT(t)
		srcDir := makeModule(t)
		dstDir := filepath.Join(t.TempDir(), "module")

		g.Expect(CopyDir(srcDir, dstDir, false)).To(Succeed())

		g.Expect(filepath.Join(dstDir, "schema.cue")).ToNot(BeAnExistingFile())
	})

	t.Run("applies ignore rules to symlinked dirs", func(t *testing.T) {
		g := NewWithT(t)
		tmpDir := t.TempDir()
		srcDir := filepath.Join(tmpDir, "module")
		g.Expect(os.MkdirAll(srcDir, 0o755)).To(Succeed())

		sharedDir := filepath.Join(tmpDir, "shared")
		g.Expect(os.MkdirAll(sharedDir, 0o755)).To(Succeed())
		g.Expect(os.WriteFile(filepath.Join(sharedDir, "keep.txt"), []byte("keep"), 0o644)).To(Succeed())
		g.Expect(os.WriteFile(filepath.Join(sharedDir, "secret.txt"), []byte("secret"), 0o644)).To(Succeed())

		// Both links materialize the same target, one is excluded by a
		// directory rule and the other has a descendant file excluded.
		g.Expect(os.Symlink(filepath.Join("..", "shared"), filepath.Join(srcDir, "linked"))).To(Succeed())
		g.Expect(os.Symlink(filepath.Join("..", "shared"), filepath.Join(srcDir, "dropped"))).To(Succeed())
		g.Expect(os.WriteFile(filepath.Join(srcDir, "timoni.ignore"),
			[]byte("dropped/\nlinked/secret.txt\n"), 0o644)).To(Succeed())

		dstDir := filepath.Join(t.TempDir(), "module")
		g.Expect(CopyDir(srcDir, dstDir, true)).To(Succeed())

		g.Expect(filepath.Join(dstDir, "linked", "keep.txt")).To(BeARegularFile())
		g.Expect(filepath.Join(dstDir, "linked", "secret.txt")).ToNot(BeAnExistingFile())
		g.Expect(filepath.Join(dstDir, "dropped")).ToNot(BeADirectory())
	})
}

func TestIsOCIUrl(t *testing.T) {
	g := NewWithT(t)
	g.Expect(IsOCIUrl("oci://foo/bar")).To(BeTrueBecause("oci:// is an OCI URL"))
	g.Expect(IsOCIUrl("file://afile.txt")).To(BeFalseBecause("file:// is not an OCI URL"))
}

func TestIsFileUrl(t *testing.T) {
	g := NewWithT(t)
	g.Expect(IsFileURL("file://afile.txt")).To(BeTrueBecause("file:// is a file URL"))
	g.Expect(IsFileURL("oci://foo/bar")).To(BeFalseBecause("oci:// is not a file URL"))
}

func TestExtractValueFromBytesLookupError(t *testing.T) {
	g := NewWithT(t)

	_, err := ExtractValueFromBytes(cuecontext.New(), []byte("other: true"), "values")
	g.Expect(err).To(MatchError(ContainSubstring("not found")))
}

func TestReadIgnoreFileScannerError(t *testing.T) {
	g := NewWithT(t)
	moduleRoot := t.TempDir()
	err := os.WriteFile(
		filepath.Join(moduleRoot, "timoni.ignore"),
		[]byte(strings.Repeat("x", bufio.MaxScanTokenSize+1)),
		0o600,
	)
	g.Expect(err).ToNot(HaveOccurred())

	_, err = ReadIgnoreFile(moduleRoot)
	g.Expect(err).To(HaveOccurred())
}
