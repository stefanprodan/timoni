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
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/mask"
)

// TestApply_DiffMasksSecrets guards the masking of Secret data in the
// server-side diff, performed by the fluxcd/pkg/ssa ResourceManager.
func TestApply_DiffMasksSecrets(t *testing.T) {
	g := NewWithT(t)
	modPath := "testdata/module-secret"
	name := rnd("my-instance")
	namespace := rnd("my-namespace")

	oldPassword := "s3cr3t"
	newPassword := "ch4ng3d"
	oldEncoded := base64.StdEncoding.EncodeToString([]byte(oldPassword))
	newEncoded := base64.StdEncoding.EncodeToString([]byte(newPassword))

	valuesPath := filepath.Join(t.TempDir(), "values.cue")
	g.Expect(os.WriteFile(valuesPath, fmt.Appendf(nil, `values: password: %q`, newPassword), 0644)).To(Succeed())

	_, err := executeCommand(fmt.Sprintf(
		"apply -n %s %s %s -p main --wait",
		namespace,
		name,
		modPath,
	))
	g.Expect(err).ToNot(HaveOccurred())

	output, err := executeCommand(fmt.Sprintf(
		"apply -n %s %s %s -p main --diff -f %s",
		namespace,
		name,
		modPath,
		valuesPath,
	))
	g.Expect(err).ToNot(HaveOccurred())
	t.Log("\n", output)

	g.Expect(output).To(ContainSubstring("Secret/%s/%s configured", namespace, name))
	g.Expect(output).To(ContainSubstring("*** (before)"))
	g.Expect(output).To(ContainSubstring("*** (after)"))
	g.Expect(output).ToNot(ContainSubstring(oldEncoded))
	g.Expect(output).ToNot(ContainSubstring(newEncoded))
	g.Expect(output).ToNot(ContainSubstring(oldPassword))
	g.Expect(output).ToNot(ContainSubstring(newPassword))
}

func TestBuild_MaskSecrets(t *testing.T) {
	modPath := "testdata/module-secret"

	t.Run("prints secret data by default", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(buildCmd.Flags().Lookup("mask-secrets").DefValue).To(Equal("false"))

		output, err := executeCommand(fmt.Sprintf(
			"build test %s -p main",
			modPath,
		))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring("password: s3cr3t"))
		g.Expect(output).To(ContainSubstring("secretName: test"))
	})

	t.Run("masks secret data in yaml and json output", func(t *testing.T) {
		g := NewWithT(t)
		for _, format := range []string{"yaml", "json"} {
			output, err := executeCommand(fmt.Sprintf(
				"build test %s -p main --mask-secrets -o %s",
				modPath,
				format,
			))
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(output).ToNot(ContainSubstring("s3cr3t"), format)
			g.Expect(output).ToNot(ContainSubstring("t0k3n"), format)
			g.Expect(output).To(ContainSubstring(mask.Value), format)
			// ConfigMap data is not masked.
			g.Expect(output).To(ContainSubstring("secretName"), format)
			g.Expect(output).To(ContainSubstring("test"), format)
		}
	})
}

func TestBundleBuild_MaskSecrets(t *testing.T) {
	g := NewWithT(t)
	modPath := "testdata/module-secret"
	namespace := rnd("my-namespace")

	bundleCue := fmt.Sprintf(`
bundle: {
	apiVersion: "v1alpha1"
	name: "secret-app"
	instances: {
		app: {
			module: url: "file://%[1]s"
			namespace: "%[2]s"
			values: password: "bundl3-p4ss"
		}
	}
}
`, modPath, namespace)

	wd := t.TempDir()
	cuePath := filepath.Join(wd, "bundle.cue")
	g.Expect(os.WriteFile(cuePath, []byte(bundleCue), 0644)).To(Succeed())
	g.Expect(engine.CopyDir(modPath, filepath.Join(wd, modPath), true)).To(Succeed())

	t.Run("prints secret data by default", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(bundleBuildCmd.Flags().Lookup("mask-secrets").DefValue).To(Equal("false"))

		output, err := executeCommand(fmt.Sprintf("bundle build -f %s -p main", cuePath))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring("password: bundl3-p4ss"))
	})

	t.Run("masks secret data on stdout", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf("bundle build -f %s -p main --mask-secrets", cuePath))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).ToNot(ContainSubstring("bundl3-p4ss"))
		g.Expect(output).ToNot(ContainSubstring("t0k3n"))
		g.Expect(output).To(ContainSubstring(mask.Value))
		g.Expect(output).To(ContainSubstring("secretName: app"))
	})

	t.Run("writes secret data to the output dir", func(t *testing.T) {
		g := NewWithT(t)
		outDir := filepath.Join(t.TempDir(), "manifests")
		_, err := executeCommand(fmt.Sprintf("bundle build -f %s -p main --mask-secrets --output-dir %s", cuePath, outDir))
		g.Expect(err).ToNot(HaveOccurred())

		data, err := os.ReadFile(filepath.Join(outDir, "app", "v1_secret_app.yaml"))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(string(data)).To(ContainSubstring("bundl3-p4ss"))
		g.Expect(string(data)).ToNot(ContainSubstring(mask.Value))
	})
}
