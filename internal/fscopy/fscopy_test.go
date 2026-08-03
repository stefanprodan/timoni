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
	"runtime"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func writeFile(t *testing.T, path, content string, perm os.FileMode) {
	t.Helper()
	g := NewWithT(t)
	g.Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
	g.Expect(os.WriteFile(path, []byte(content), perm)).To(Succeed())
}

func TestCopyDir(t *testing.T) {
	g := NewWithT(t)
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")

	writeFile(t, filepath.Join(src, "a.txt"), "a", 0o600)
	writeFile(t, filepath.Join(src, "sub", "b.txt"), "b", 0o644)
	g.Expect(os.MkdirAll(filepath.Join(src, "empty"), 0o755)).To(Succeed())

	g.Expect(CopyDir(src, dst, Options{})).To(Succeed())

	data, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(data)).To(Equal("a"))
	g.Expect(filepath.Join(dst, "sub", "b.txt")).To(BeARegularFile())
	g.Expect(filepath.Join(dst, "empty")).To(BeADirectory())

	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dst, "a.txt"))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
	}
}

func TestCopyDir_Skip(t *testing.T) {
	g := NewWithT(t)
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")

	writeFile(t, filepath.Join(src, "keep.txt"), "keep", 0o644)
	writeFile(t, filepath.Join(src, "drop.txt"), "drop", 0o644)
	writeFile(t, filepath.Join(src, "dropdir", "nested.txt"), "nested", 0o644)

	skip := func(path string, isDir bool) bool {
		return strings.HasPrefix(filepath.Base(path), "drop")
	}
	g.Expect(CopyDir(src, dst, Options{Skip: skip})).To(Succeed())

	g.Expect(filepath.Join(dst, "keep.txt")).To(BeARegularFile())
	g.Expect(filepath.Join(dst, "drop.txt")).ToNot(BeAnExistingFile())
	g.Expect(filepath.Join(dst, "dropdir")).ToNot(BeADirectory())
}

func TestCopyDir_SymlinkDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	g := NewWithT(t)
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")

	writeFile(t, filepath.Join(src, "real.txt"), "real", 0o644)
	g.Expect(os.Symlink("real.txt", filepath.Join(src, "link.txt"))).To(Succeed())
	g.Expect(os.Symlink("../outside", filepath.Join(src, "dangling"))).To(Succeed())

	g.Expect(CopyDir(src, dst, Options{})).To(Succeed())

	// Symlinks are skipped, whether their target exists or not.
	g.Expect(filepath.Join(dst, "real.txt")).To(BeARegularFile())
	_, err := os.Lstat(filepath.Join(dst, "link.txt"))
	g.Expect(os.IsNotExist(err)).To(BeTrue())
	_, err = os.Lstat(filepath.Join(dst, "dangling"))
	g.Expect(os.IsNotExist(err)).To(BeTrue())
}

func TestCopyDir_FollowSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	g := NewWithT(t)
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")

	// A shared directory outside the source tree, linked relatively,
	// mirroring vendored CUE schemas shared across monorepo modules.
	writeFile(t, filepath.Join(tmp, "shared", "schema.cue"), "schema", 0o644)
	writeFile(t, filepath.Join(src, "values.cue"), "values", 0o644)
	g.Expect(os.MkdirAll(filepath.Join(src, "cue.mod"), 0o755)).To(Succeed())
	g.Expect(os.Symlink(filepath.Join("..", "..", "shared"),
		filepath.Join(src, "cue.mod", "pkg"))).To(Succeed())
	g.Expect(os.Symlink("values.cue", filepath.Join(src, "link.cue"))).To(Succeed())

	g.Expect(CopyDir(src, dst, Options{FollowSymlinks: true})).To(Succeed())

	// Both the linked directory and the linked file are materialized.
	g.Expect(filepath.Join(dst, "cue.mod", "pkg", "schema.cue")).To(BeARegularFile())
	g.Expect(filepath.Join(dst, "link.cue")).To(BeARegularFile())
	info, err := os.Lstat(filepath.Join(dst, "cue.mod", "pkg"))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(info.Mode() & os.ModeSymlink).To(BeZero())
}

func TestCopyDir_FollowSymlinksSharedTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	g := NewWithT(t)
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")

	// Two sibling links to the same target form a diamond, not a cycle.
	writeFile(t, filepath.Join(tmp, "shared", "file.txt"), "shared", 0o644)
	g.Expect(os.MkdirAll(src, 0o755)).To(Succeed())
	g.Expect(os.Symlink(filepath.Join("..", "shared"), filepath.Join(src, "one"))).To(Succeed())
	g.Expect(os.Symlink(filepath.Join("..", "shared"), filepath.Join(src, "two"))).To(Succeed())

	g.Expect(CopyDir(src, dst, Options{FollowSymlinks: true})).To(Succeed())

	g.Expect(filepath.Join(dst, "one", "file.txt")).To(BeARegularFile())
	g.Expect(filepath.Join(dst, "two", "file.txt")).To(BeARegularFile())
}

func TestCopyDir_FollowSymlinksCycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	g := NewWithT(t)
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")

	// A link pointing back to the source root forms a cycle.
	g.Expect(os.MkdirAll(filepath.Join(src, "sub"), 0o755)).To(Succeed())
	g.Expect(os.Symlink("..", filepath.Join(src, "sub", "loop"))).To(Succeed())

	err := CopyDir(src, dst, Options{FollowSymlinks: true})
	g.Expect(err).To(MatchError(ErrSymlinkCycle))
}

func TestCopyDir_FollowSymlinksDangling(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	g := NewWithT(t)
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")

	g.Expect(os.MkdirAll(src, 0o755)).To(Succeed())
	g.Expect(os.Symlink("missing", filepath.Join(src, "dangling"))).To(Succeed())

	err := CopyDir(src, dst, Options{FollowSymlinks: true})
	g.Expect(err).To(MatchError(ContainSubstring("resolving symlink")))
}

func TestCopyDir_MaxEntries(t *testing.T) {
	g := NewWithT(t)
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")

	for _, name := range []string{"a", "b", "c"} {
		writeFile(t, filepath.Join(src, name), name, 0o644)
	}

	err := CopyDir(src, dst, Options{MaxEntries: 2})
	g.Expect(err).To(MatchError(ErrTooManyEntries))
}

func TestCopyDir_InvalidArgs(t *testing.T) {
	g := NewWithT(t)
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	g.Expect(os.MkdirAll(src, 0o755)).To(Succeed())

	err := CopyDir(src, filepath.Join(src, "dst"), Options{})
	g.Expect(err).To(MatchError(ContainSubstring("inside source")))

	err = CopyDir(src, src, Options{})
	g.Expect(err).To(MatchError(ContainSubstring("inside source")))

	file := filepath.Join(tmp, "file.txt")
	writeFile(t, file, "x", 0o644)
	err = CopyDir(file, filepath.Join(tmp, "dst"), Options{})
	g.Expect(err).To(MatchError(ContainSubstring("not a directory")))

	err = CopyDir(filepath.Join(tmp, "missing"), filepath.Join(tmp, "dst"), Options{})
	g.Expect(err).To(HaveOccurred())
}

func TestCopyFile(t *testing.T) {
	g := NewWithT(t)
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.txt")
	dst := filepath.Join(tmp, "nested", "dir", "dst.txt")

	writeFile(t, src, "content", 0o600)

	g.Expect(CopyFile(src, dst)).To(Succeed())

	data, err := os.ReadFile(dst)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(data)).To(Equal("content"))

	if runtime.GOOS != "windows" {
		info, err := os.Stat(dst)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
	}

	err = CopyFile(tmp, filepath.Join(tmp, "dir-as-src.txt"))
	g.Expect(err).To(MatchError(ContainSubstring("not a regular file")))

	// Copying a file onto itself is rejected instead of truncating it.
	err = CopyFile(src, src)
	g.Expect(err).To(MatchError(ContainSubstring("same file")))
	data, err = os.ReadFile(src)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(data)).To(Equal("content"))
}
