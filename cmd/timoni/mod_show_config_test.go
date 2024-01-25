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

package main

import (
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/gomega"
)

func Test_ShowConfig(t *testing.T) {
	modPath := "testdata/module"

	g := NewWithT(t)

	// Push the module to registry
	output, err := executeCommand(fmt.Sprintf(
		"mod show config %s",
		modPath,
	))
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(output).To(ContainSubstring("`client: enabled:`"))
	g.Expect(output).To(ContainSubstring("`client: image: repository:`"))
	g.Expect(output).To(ContainSubstring("`server: enabled:`"))
}

func Test_ShowConfigOutput(t *testing.T) {
	modPath := "testdata/module"
	filePath := fmt.Sprintf("%s/README.md", modPath)

	g := NewWithT(t)

	// Push the module to registry
	_, err := executeCommand(fmt.Sprintf(
		"mod show config %s --output %s",
		modPath,
		filePath,
	))
	g.Expect(err).ToNot(HaveOccurred())

	rmFile, err := os.ReadFile(filePath)
	g.Expect(err).ToNot(HaveOccurred())

	strContent := string(rmFile)

	g.Expect(strContent).To(ContainSubstring("# module"))
	g.Expect(strContent).To(ContainSubstring("## Install"))
	g.Expect(strContent).To(ContainSubstring("## Uninstall"))
	g.Expect(strContent).To(ContainSubstring("## Configuration"))
	g.Expect(strContent).To(ContainSubstring("`client: enabled:`"))
	g.Expect(strContent).To(ContainSubstring("`client: image: repository:`"))
	g.Expect(strContent).To(ContainSubstring("`server: enabled:`"))

	g.Expect(err).ToNot(HaveOccurred())
}

func Test_ShowConfigOutputNewFile(t *testing.T) {
	modPath := "testdata/module"
	filePath := fmt.Sprintf("%s/testing.md", t.TempDir())

	g := NewWithT(t)

	// Push the module to registry
	_, err := executeCommand(fmt.Sprintf(
		"mod show config %s --output %s",
		modPath,
		filePath,
	))
	g.Expect(err).ToNot(HaveOccurred())

	rmFile, err := os.ReadFile(filePath)
	g.Expect(err).ToNot(HaveOccurred())

	strContent := string(rmFile)

	g.Expect(strContent).To(ContainSubstring("`client: enabled:`"))
	g.Expect(strContent).To(ContainSubstring("`client: image: repository:`"))
	g.Expect(strContent).To(ContainSubstring("`server: enabled:`"))
}
