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
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func Test_ListArtifact(t *testing.T) {
	g := NewWithT(t)
	aPath := "testdata/module-values"
	aURL := fmt.Sprintf("%s/%s", dockerRegistry, rnd("my-artifact", 5))
	aTags := []string{"1.0.0", "latest"}

	_, err := executeCommand(fmt.Sprintf(
		"artifact push oci://%s -f %s -t %s -t %s",
		aURL,
		aPath,
		aTags[0],
		aTags[1],
	))
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("prints table", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"artifact ls oci://%s",
			aURL,
		))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(output).To(ContainSubstring("TAG"))
		for _, tag := range aTags {
			g.Expect(output).To(ContainSubstring(tag))
		}
	})

	t.Run("prints JSON", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"artifact ls oci://%s -o json",
			aURL,
		))
		g.Expect(err).ToNot(HaveOccurred())

		var list []apiv1.ArtifactReference
		g.Expect(json.Unmarshal([]byte(output), &list)).To(Succeed())

		tags := map[string]bool{}
		for _, ref := range list {
			g.Expect(ref.Repository).To(ContainSubstring(aURL))
			g.Expect(ref.Digest).ToNot(BeEmpty())
			tags[ref.Tag] = true
		}
		for _, tag := range aTags {
			g.Expect(tags).To(HaveKey(tag))
		}
	})

	t.Run("prints YAML", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"artifact ls oci://%s -o yaml",
			aURL,
		))
		g.Expect(err).ToNot(HaveOccurred())

		var list []apiv1.ArtifactReference
		g.Expect(yaml.Unmarshal([]byte(output), &list)).To(Succeed())

		tags := map[string]bool{}
		for _, ref := range list {
			g.Expect(ref.Digest).ToNot(BeEmpty())
			tags[ref.Tag] = true
		}
		for _, tag := range aTags {
			g.Expect(tags).To(HaveKey(tag))
		}
	})

	t.Run("fails for invalid output format", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand("artifact ls oci://registry.example.com/org/app -o junk")
		g.Expect(err).To(MatchError("unknown --output=junk, can be yaml or json"))
	})
}
