//go:build !windows

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

package fscopy

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	. "github.com/onsi/gomega"
)

func TestCopyDir_SpecialFiles(t *testing.T) {
	g := NewWithT(t)
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")

	writeFile(t, filepath.Join(src, "regular.txt"), "x", 0o644)
	g.Expect(syscall.Mkfifo(filepath.Join(src, "pipe"), 0o644)).To(Succeed())

	g.Expect(CopyDir(src, dst, Options{})).To(Succeed())

	g.Expect(filepath.Join(dst, "regular.txt")).To(BeARegularFile())
	g.Expect(filepath.Join(dst, "pipe")).ToNot(BeAnExistingFile())
}

func TestCopyDir_DestinationAlias(t *testing.T) {
	g := NewWithT(t)
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	writeFile(t, filepath.Join(src, "a.txt"), "a", 0o644)

	// The destination itself is a symlink pointing inside the source.
	g.Expect(os.Symlink(src, filepath.Join(tmp, "alias"))).To(Succeed())
	err := CopyDir(src, filepath.Join(tmp, "alias"), Options{})
	g.Expect(err).To(MatchError(ContainSubstring("inside source")))

	// A non-existing destination under a symlinked ancestor
	// resolving inside the source is rejected as well.
	err = CopyDir(src, filepath.Join(tmp, "alias", "sub"), Options{})
	g.Expect(err).To(MatchError(ContainSubstring("inside source")))

	// The source is unharmed.
	data, err := os.ReadFile(filepath.Join(src, "a.txt"))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(data)).To(Equal("a"))
}

func TestCopyFile_HardLink(t *testing.T) {
	g := NewWithT(t)
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.txt")
	writeFile(t, src, "content", 0o644)

	link := filepath.Join(tmp, "hardlink.txt")
	g.Expect(os.Link(src, link)).To(Succeed())

	err := CopyFile(src, link)
	g.Expect(err).To(MatchError(ContainSubstring("same file")))

	data, err := os.ReadFile(src)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(data)).To(Equal("content"))
}

func TestCopyDir_FollowSymlinksSkipDir(t *testing.T) {
	g := NewWithT(t)
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")

	// A directory symlink must be skippable via directory rules,
	// and Skip must see materialized content at its logical path
	// under the source root, not at the physical target location.
	writeFile(t, filepath.Join(tmp, "shared", "file.txt"), "x", 0o644)
	writeFile(t, filepath.Join(src, "keep.txt"), "x", 0o644)
	g.Expect(os.Symlink(filepath.Join("..", "shared"), filepath.Join(src, "linked"))).To(Succeed())
	g.Expect(os.Symlink(filepath.Join("..", "shared"), filepath.Join(src, "dropped"))).To(Succeed())

	var seen []string
	opts := Options{
		FollowSymlinks: true,
		Skip: func(path string, isDir bool) bool {
			seen = append(seen, path)
			return isDir && filepath.Base(path) == "dropped"
		},
	}
	g.Expect(CopyDir(src, dst, opts)).To(Succeed())

	g.Expect(filepath.Join(dst, "linked", "file.txt")).To(BeARegularFile())
	g.Expect(filepath.Join(dst, "dropped")).ToNot(BeADirectory())

	srcReal, err := filepath.EvalSymlinks(src)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(seen).To(ContainElement(filepath.Join(srcReal, "linked", "file.txt")))
	for _, p := range seen {
		g.Expect(p).To(HavePrefix(srcReal+string(filepath.Separator)),
			"Skip should only see logical paths under the source root")
	}
}
