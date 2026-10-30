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

package oci

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/stefanprodan/timoni/internal/fscopy"
)

func TestCopyArtifactFileReturnsOpenError(t *testing.T) {
	g := NewWithT(t)

	err := copyArtifactFile(io.Discard, filepath.Join(t.TempDir(), "missing"))
	g.Expect(err).To(HaveOccurred())
}

func TestNormalizeMode(t *testing.T) {
	tests := []struct {
		mode os.FileMode
		want int64
	}{
		{fs.ModeDir | 0o600, 0o755},
		{fs.ModeDir | 0o700, 0o755},
		{fs.ModeDir | 0o775, 0o755},
		{fs.ModeDir | fs.ModeSetgid | fs.ModeSticky | 0o755, 0o755},
		{0o644, 0o644},
		{0o664, 0o644},
		{0o600, 0o644},
		{0o444, 0o644},
		{0o666, 0o644},
		{0o654, 0o644},
		{0o645, 0o644},
		{0o611, 0o644},
		{0o700, 0o755},
		{0o744, 0o755},
		{0o750, 0o755},
		{fs.ModeSetuid | 0o755, 0o755},
	}
	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(normalizeMode(tt.mode)).To(Equal(tt.want))
		})
	}
}

func TestBuildArtifactNormalizesModes(t *testing.T) {
	skipOnWindows(t)
	g := NewWithT(t)

	src := t.TempDir()
	writeTree(t, src, map[string]os.FileMode{
		".":          fs.ModeDir | 0o775,
		"sub":        fs.ModeDir | 0o700,
		"rw.txt":     0o664,
		"private":    0o600,
		"run.sh":     0o750,
		"owner.sh":   0o744,
		"group-x":    0o654,
		"setuid.sh":  fs.ModeSetuid | 0o755,
		"sub/nested": 0o640,
	})

	modes := readArchiveModes(t, buildTestArtifact(t, src))
	g.Expect(modes).To(Equal(map[string]int64{
		".":          0o755,
		"sub":        0o755,
		"rw.txt":     0o644,
		"private":    0o644,
		"run.sh":     0o755,
		"owner.sh":   0o755,
		"group-x":    0o644,
		"setuid.sh":  0o755,
		"sub/nested": 0o644,
	}))
}

func TestBuildArtifactNormalizesSingleFile(t *testing.T) {
	skipOnWindows(t)
	tests := []struct {
		mode os.FileMode
		want int64
	}{
		{0o750, 0o755},
		{0o640, 0o644},
	}
	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			g := NewWithT(t)
			src := t.TempDir()
			writeTree(t, src, map[string]os.FileMode{"file": tt.mode})

			modes := readArchiveModes(t, buildTestArtifact(t, filepath.Join(src, "file")))
			g.Expect(modes).To(Equal(map[string]int64{"file": tt.want}))
		})
	}
}

func TestBuildArtifactDigestIgnoresUmaskModes(t *testing.T) {
	skipOnWindows(t)
	g := NewWithT(t)

	// Modes produced by umask 0022, 0002 and 0027.
	variants := []struct{ dir, file, exec os.FileMode }{
		{0o755, 0o644, 0o755},
		{0o775, 0o664, 0o775},
		{0o750, 0o640, 0o750},
	}

	archives := make([][]byte, 0, len(variants))
	for _, v := range variants {
		src := t.TempDir()
		writeTree(t, src, map[string]os.FileMode{
			".":          fs.ModeDir | v.dir,
			"sub":        fs.ModeDir | v.dir,
			"main.cue":   v.file,
			"sub/run.sh": v.exec,
		})
		data, err := os.ReadFile(buildTestArtifact(t, src))
		g.Expect(err).ToNot(HaveOccurred())
		archives = append(archives, data)
	}

	g.Expect(archives[1]).To(Equal(archives[0]))
	g.Expect(archives[2]).To(Equal(archives[0]))
}

func TestBuildArtifactKeepsOwnerExecuteBit(t *testing.T) {
	skipOnWindows(t)
	g := NewWithT(t)

	exec := t.TempDir()
	writeTree(t, exec, map[string]os.FileMode{".": fs.ModeDir | 0o755, "file": 0o744})
	plain := t.TempDir()
	writeTree(t, plain, map[string]os.FileMode{".": fs.ModeDir | 0o755, "file": 0o644})

	g.Expect(readArchiveModes(t, buildTestArtifact(t, exec))).To(HaveKeyWithValue("file", int64(0o755)))
	g.Expect(readArchiveModes(t, buildTestArtifact(t, plain))).To(HaveKeyWithValue("file", int64(0o644)))
}

func TestBuildModuleImageNormalizesModes(t *testing.T) {
	skipOnWindows(t)
	g := NewWithT(t)
	annotations := map[string]string{"example.com/key": "value"}

	variants := []struct{ dir, file os.FileMode }{{0o755, 0o644}, {0o750, 0o640}}
	digests := make([]string, 0, len(variants))
	for _, mode := range variants {
		src := filepath.Join(t.TempDir(), "module")
		g.Expect(fscopy.CopyDir("testdata/module", src, fscopy.Options{})).To(Succeed())
		g.Expect(filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return os.Chmod(p, mode.dir)
			}
			return os.Chmod(p, mode.file)
		})).To(Succeed())

		build, err := BuildModuleImage(src, []string{"timoni.ignore"}, annotations)
		g.Expect(err).ToNot(HaveOccurred())
		digest, err := build.Image.Digest()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(build.Close()).To(Succeed())
		digests = append(digests, digest.String())
	}

	g.Expect(digests[1]).To(Equal(digests[0]))
}

// skipOnWindows skips the test on Windows.
func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not supported on Windows")
	}
}

// writeTree creates the entries under root and sets their modes. Entries
// with fs.ModeDir set are directories, "." being root itself. Directory
// modes are set after all entries are created.
func writeTree(t *testing.T, root string, entries map[string]os.FileMode) {
	t.Helper()
	g := NewWithT(t)
	for name, mode := range entries {
		p := filepath.Join(root, name)
		if mode.IsDir() {
			g.Expect(os.MkdirAll(p, 0o755)).To(Succeed())
			continue
		}
		g.Expect(os.MkdirAll(filepath.Dir(p), 0o755)).To(Succeed())
		g.Expect(os.WriteFile(p, []byte(name), 0o600)).To(Succeed())
		g.Expect(os.Chmod(p, mode)).To(Succeed())
	}
	for name, mode := range entries {
		if mode.IsDir() {
			g.Expect(os.Chmod(filepath.Join(root, name), mode.Perm())).To(Succeed())
		}
	}
}

// buildTestArtifact builds src into a tgz file and returns its path.
func buildTestArtifact(t *testing.T, src string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "artifact.tgz")
	NewWithT(t).Expect(BuildArtifact(dst, src, nil)).To(Succeed())
	return dst
}

// readArchiveModes returns the header mode of every entry in the tgz file.
func readArchiveModes(t *testing.T, path string) map[string]int64 {
	t.Helper()
	g := NewWithT(t)
	f, err := os.Open(path)
	g.Expect(err).ToNot(HaveOccurred())
	defer f.Close()
	gr, err := gzip.NewReader(f)
	g.Expect(err).ToNot(HaveOccurred())
	tr := tar.NewReader(gr)

	modes := map[string]int64{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		g.Expect(err).ToNot(HaveOccurred())
		modes[h.Name] = h.Mode
	}
	return modes
}
