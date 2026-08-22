/*
Copyright 2026 Stefan Prodan
SPDX-License-Identifier: Apache-2.0

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
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func Test_ShowReadme(t *testing.T) {
	g := NewWithT(t)

	output, err := executeCommand("mod show readme testdata/module")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(ContainSubstring("## Configuration"))

	modDir := filepath.Join(t.TempDir(), "module")
	g.Expect(exec.Command("cp", "-r", "testdata/module", modDir).Run()).To(Succeed())
	g.Expect(os.Remove(filepath.Join(modDir, "README.md"))).To(Succeed())

	_, err = executeCommand(fmt.Sprintf("mod show readme %s", modDir))
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("reading README.md"))
}

func Test_ShowReadmeOCI(t *testing.T) {
	g := NewWithT(t)
	modURL := fmt.Sprintf("%s/%s", dockerRegistry, rnd("my-mod"))
	modVer := "1.0.0"

	_, err := executeCommand(fmt.Sprintf(
		"mod push testdata/module oci://%s -v %s --resolve-symlinks",
		modURL,
		modVer,
	))
	g.Expect(err).ToNot(HaveOccurred())

	output, err := executeCommand(fmt.Sprintf(
		"mod show readme oci://%s -v %s",
		modURL,
		modVer,
	))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(ContainSubstring("## Configuration"))
}
