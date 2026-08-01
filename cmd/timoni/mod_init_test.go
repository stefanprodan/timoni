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
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestInitModuleFromTemplateRejectsSymlinkAndCleansDestination(t *testing.T) {
	g := NewWithT(t)
	src := t.TempDir()
	g.Expect(os.WriteFile(filepath.Join(src, "a.txt"), []byte("module"), 0o600)).To(Succeed())
	if err := os.Symlink("a.txt", filepath.Join(src, "z-link")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "module")

	err := initModuleFromTemplate("module", "template", src, dst)

	g.Expect(err).To(MatchError(ContainSubstring("template entry is not a regular file")))
	_, statErr := os.Stat(dst)
	g.Expect(os.IsNotExist(statErr)).To(BeTrue())
}

func TestInitModuleFromTemplatePreservesExistingDestination(t *testing.T) {
	g := NewWithT(t)
	src := t.TempDir()
	dst := t.TempDir()
	sentinel := filepath.Join(dst, "sentinel")
	g.Expect(os.WriteFile(sentinel, []byte("keep"), 0o600)).To(Succeed())

	err := initModuleFromTemplate("module", "template", src, dst)

	g.Expect(err).To(MatchError(ContainSubstring("already exists")))
	g.Expect(sentinel).To(BeAnExistingFile())
}

func TestInitializeModuleCleansDestinationAfterIgnoreWriteError(t *testing.T) {
	g := NewWithT(t)
	src := t.TempDir()
	g.Expect(os.Mkdir(filepath.Join(src, apiv1.IgnoreFile), 0o700)).To(Succeed())
	dst := filepath.Join(t.TempDir(), "module")

	err := initializeModule("module", "template", src, dst)

	g.Expect(err).To(HaveOccurred())
	_, statErr := os.Stat(dst)
	g.Expect(os.IsNotExist(statErr)).To(BeTrue())
}
