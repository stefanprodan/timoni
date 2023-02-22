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

package main

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
)

func TestInspect(t *testing.T) {
	g := NewWithT(t)
	modPath := "testdata/cs"
	modURL := fmt.Sprintf("oci://%s/%s", dockerRegistry, rnd("my-mod", 5))
	modVer := "1.0.0"
	name := rnd("my-instance", 5)
	namespace := rnd("my-namespace", 5)

	// Package the module as an OCI artifact and push it to registry
	_, err := executeCommand(fmt.Sprintf(
		"mod push %s %s -v %s",
		modPath,
		modURL,
		modVer,
	))
	g.Expect(err).ToNot(HaveOccurred())

	// Install the module from the registry
	_, err = executeCommand(fmt.Sprintf(
		"apply -n %s %s %s -v %s -p main --wait",
		namespace,
		name,
		modURL,
		modVer,
	))
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("inspect module", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"inspect module -n %s %s",
			namespace,
			name,
		))
		g.Expect(err).ToNot(HaveOccurred())

		// Verify inspect output contains the module metadata
		g.Expect(output).To(ContainSubstring(modURL))
		g.Expect(output).To(ContainSubstring(modVer))
		g.Expect(output).To(ContainSubstring("sha256"))
		g.Expect(output).To(ContainSubstring("timoni.sh/cs"))
	})

	t.Run("inspect values", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"inspect values -n %s %s",
			namespace,
			name,
		))
		g.Expect(err).ToNot(HaveOccurred())

		// Verify inspect output contains the expected values
		g.Expect(output).To(ContainSubstring("example.internal"))
	})

	t.Run("inspect resources", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"inspect resources -n %s %s",
			namespace,
			name,
		))
		g.Expect(err).ToNot(HaveOccurred())

		// Verify inspect output contains the expected resources
		g.Expect(output).To(ContainSubstring(fmt.Sprintf("ConfigMap/%s/%s-client", namespace, name)))
	})
}
