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

package fetcher

import (
	"path/filepath"
	"testing"

	cp "github.com/otiai10/copy"

	. "github.com/stefanprodan/timoni/internal/testutils"
)

func TestNewLocal(t *testing.T) {
	g := NewWithT(t)
	lf := NewLocal("src", "dst")

	g.Expect(lf).ToNot(BeNil())
	g.Expect(lf).To(Implement((*Fetcher)(nil)))
}

func TestLocalGetModuleRoot(t *testing.T) {
	g := NewWithT(t)
	lf := NewLocal("src", "dst")

	g.Expect(lf.GetModuleRoot()).To(Equal("dst/module"))
}

func TestLocalFetch(t *testing.T) {
	t.Run("nominal", func(t *testing.T) {
		g := NewWithT(t)
		src := filepath.Join(t.TempDir(), "src")
		dst := filepath.Join(t.TempDir(), "dst")
		testmod := "testdata/module"

		g.Expect(cp.Copy(testmod, src)).To(Succeed())

		lf := NewLocal(src, dst)
		mr, err := lf.Fetch()

		g.Expect(err).To(BeNil())
		g.Expect(mr.Repository).To(Equal(src))
		g.Expect(filepath.Join(lf.GetModuleRoot(), "cue.mod/module.cue")).To(BeARegularFile())
	})

	t.Run("lack of required files", func(t *testing.T) {
		g := NewWithT(t)
		src := t.TempDir()
		dst := filepath.Join(t.TempDir(), "dst")

		lf := NewLocal(src, dst)
		_, err := lf.Fetch()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("required file not found:"))
	})

	t.Run("non existent source", func(t *testing.T) {
		g := NewWithT(t)
		lf := NewLocal("", "dst")
		_, err := lf.Fetch()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("module not found at path"))
	})
}
