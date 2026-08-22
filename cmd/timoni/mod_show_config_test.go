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
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	g.Expect(output).To(HavePrefix("#Config: {"))
	g.Expect(output).To(ContainSubstring("enabled: *true | bool"))
	g.Expect(output).To(ContainSubstring("repository: *\"cgr.dev/chainguard/timoni\" | string"))
	g.Expect(output).To(ContainSubstring("server: enabled: *true | bool"))
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

func Test_ShowConfigOutputScannerError(t *testing.T) {
	g := NewWithT(t)
	filePath := fmt.Sprintf("%s/README.md", t.TempDir())
	original := []byte(strings.Repeat("x", bufio.MaxScanTokenSize+1))
	g.Expect(os.WriteFile(filePath, original, 0o600)).To(Succeed())

	_, err := executeCommand(fmt.Sprintf(
		"mod show config testdata/module --output %s",
		filePath,
	))
	g.Expect(err).To(HaveOccurred())
	contents, readErr := os.ReadFile(filePath)
	g.Expect(readErr).ToNot(HaveOccurred())
	g.Expect(contents).To(Equal(original))
	_, statErr := os.Stat(filePath + ".tmp")
	g.Expect(os.IsNotExist(statErr)).To(BeTrue())
}

func Test_ShowConfigMarkers(t *testing.T) {
	g := NewWithT(t)

	output, err := executeCommand("mod show config testdata/module")
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(output).To(ContainSubstring("// Annotations applied to pods\n\tpodAnnotations?: {[string]: string}"))
	g.Expect(output).To(ContainSubstring("team: \"test\"\n"))
	g.Expect(output).To(ContainSubstring("domain: *\"example.internal\" | string"))
	g.Expect(output).ToNot(ContainSubstring("kubeVersion!"))
	g.Expect(output).ToNot(ContainSubstring("moduleVersion!"))

	// The Markdown table is written only with --output.
	filePath := filepath.Join(t.TempDir(), "config.md")
	_, err = executeCommand(fmt.Sprintf("mod show config testdata/module --output %s", filePath))
	g.Expect(err).ToNot(HaveOccurred())
	content, err := os.ReadFile(filePath)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(content)).To(ContainSubstring("| `podAnnotations?:`"))
	g.Expect(string(content)).To(ContainSubstring("| `team:`"))
	g.Expect(string(content)).ToNot(ContainSubstring("| `client:`"))
}

func Test_ShowConfigOCI(t *testing.T) {
	g := NewWithT(t)
	modPath := "testdata/module"
	modURL := fmt.Sprintf("%s/%s", dockerRegistry, rnd("my-mod"))
	modVer := "1.0.0"

	_, err := executeCommand(fmt.Sprintf(
		"mod push %s oci://%s -v %s --resolve-symlinks",
		modPath,
		modURL,
		modVer,
	))
	g.Expect(err).ToNot(HaveOccurred())

	output, err := executeCommand(fmt.Sprintf(
		"mod show config oci://%s -v %s",
		modURL,
		modVer,
	))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(HavePrefix("#Config: {"))
	g.Expect(output).To(ContainSubstring("podAnnotations?: {[string]: string}"))

	_, err = executeCommand(fmt.Sprintf(
		"mod show config oci://%s -v %s --output %s",
		modURL,
		modVer,
		filepath.Join(t.TempDir(), "README.md"),
	))
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("not supported for OCI modules"))
}
