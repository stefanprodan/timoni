/*
Copyright 2024 Stefan Prodan

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

package fetcher

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/stefanprodan/timoni/internal/testutils"
)

func TestNewLocal(t *testing.T) {
	g := NewWithT(t)
	lf := NewLocal("src")

	g.Expect(lf).ToNot(BeNil())
	g.Expect(lf).To(Implement((*Fetcher)(nil)))
}

func TestLocalGetModuleRoot(t *testing.T) {
	g := NewWithT(t)
	cwd, err := os.Getwd()
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("resolves relative source", func(t *testing.T) {
		g := NewWithT(t)
		lf := NewLocal("src")

		g.Expect(lf.GetModuleRoot()).To(Equal(filepath.Join(cwd, "src")))
	})

	t.Run("strips the file scheme", func(t *testing.T) {
		g := NewWithT(t)
		lf := NewLocal("file://src")

		g.Expect(lf.GetModuleRoot()).To(Equal(filepath.Join(cwd, "src")))
	})
}

func TestLocalFetch(t *testing.T) {
	t.Run("nominal", func(t *testing.T) {
		g := NewWithT(t)
		src := "testdata/module"

		lf := NewLocal(src)
		mr, err := lf.Fetch()

		g.Expect(err).To(BeNil())
		g.Expect(mr.Repository).To(Equal(src))
		g.Expect(filepath.Join(lf.GetModuleRoot(), "cue.mod/module.cue")).To(BeARegularFile())
	})

	t.Run("builds in place without applying timoni.ignore", func(t *testing.T) {
		g := NewWithT(t)
		src := "testdata/module"

		lf := NewLocal(src)
		_, err := lf.Fetch()

		g.Expect(err).To(BeNil())
		g.Expect(filepath.Join(lf.GetModuleRoot(), "timoni.ignore")).To(BeARegularFile())
		g.Expect(filepath.Join(lf.GetModuleRoot(), "ignore/ignore.txt")).To(BeARegularFile())
	})

	t.Run("resolves source at construction time", func(t *testing.T) {
		g := NewWithT(t)
		lf := NewLocal("testdata/module")
		t.Chdir(t.TempDir())

		mr, err := lf.Fetch()

		g.Expect(err).To(BeNil())
		g.Expect(mr.Repository).To(Equal("testdata/module"))
	})

	t.Run("lack of required files", func(t *testing.T) {
		g := NewWithT(t)
		src := t.TempDir()

		lf := NewLocal(src)
		_, err := lf.Fetch()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("required file not found:"))
	})

	t.Run("non existent source", func(t *testing.T) {
		g := NewWithT(t)
		lf := NewLocal("")
		_, err := lf.Fetch()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("module not found at path"))
	})
}
