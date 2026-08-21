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

	"github.com/google/go-containerregistry/pkg/crane"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func Test_DigestArtifact(t *testing.T) {
	g := NewWithT(t)
	aPath := "testdata/module-values"
	aURL := fmt.Sprintf("%s/%s", dockerRegistry, rnd("my-artifact"))

	_, err := executeCommand(fmt.Sprintf("artifact push oci://%s -f %s -t 1.0.0 -t latest", aURL, aPath))
	g.Expect(err).ToNot(HaveOccurred())

	latestDigest, err := crane.Digest(fmt.Sprintf("%s:latest", aURL))
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("defaults to the latest tag", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf("artifact digest oci://%s", aURL))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(output).To(ContainSubstring(fmt.Sprintf("artifact: oci://%s:latest", aURL)))
		g.Expect(output).To(ContainSubstring(fmt.Sprintf("digest: %s", latestDigest)))
	})

	t.Run("resolves the specified tag", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf("artifact digest oci://%s:1.0.0", aURL))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(output).To(ContainSubstring(fmt.Sprintf("artifact: oci://%s:1.0.0", aURL)))
		g.Expect(output).To(ContainSubstring(fmt.Sprintf("digest: %s", latestDigest)))
	})

	t.Run("prints JSON", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf("artifact digest oci://%s:1.0.0 -o json", aURL))
		g.Expect(err).ToNot(HaveOccurred())

		var ref apiv1.ArtifactReference
		g.Expect(json.Unmarshal([]byte(output), &ref)).To(Succeed())
		g.Expect(ref.Repository).To(Equal(fmt.Sprintf("oci://%s", aURL)))
		g.Expect(ref.Tag).To(Equal("1.0.0"))
		g.Expect(ref.Digest).To(Equal(latestDigest))
	})

	t.Run("prints YAML", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf("artifact digest oci://%s -o yaml", aURL))
		g.Expect(err).ToNot(HaveOccurred())

		var ref apiv1.ArtifactReference
		g.Expect(yaml.Unmarshal([]byte(output), &ref)).To(Succeed())
		g.Expect(ref.Repository).To(Equal(fmt.Sprintf("oci://%s", aURL)))
		g.Expect(ref.Tag).To(Equal("latest"))
		g.Expect(ref.Digest).To(Equal(latestDigest))
	})

	t.Run("fails for missing tag", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand(fmt.Sprintf("artifact digest oci://%s:2.0.0", aURL))
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("resolving digest"))
	})

	t.Run("fails for invalid URL", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand(fmt.Sprintf("artifact digest %s", aURL))
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("URL must be in format"))
	})

	t.Run("fails for invalid output format", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand(fmt.Sprintf("artifact digest oci://%s -o junk", aURL))
		g.Expect(err).To(MatchError("unknown --output=junk, can be yaml or json"))
	})
}
