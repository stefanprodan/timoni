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
	aURL := fmt.Sprintf("%s/%s", dockerRegistry, rnd("my-artifact"))
	aTags := []string{"1.0.0", "1.1.0", "2.0.0", "dev", "latest"}

	pushCmd := fmt.Sprintf("artifact push oci://%s -f %s", aURL, aPath)
	for _, tag := range aTags {
		pushCmd += fmt.Sprintf(" -t %s", tag)
	}

	_, err := executeCommand(pushCmd)
	g.Expect(err).ToNot(HaveOccurred())

	listTags := func(t *testing.T, args string) []string {
		t.Helper()
		g := NewWithT(t)

		output, _, err := executeCommandWithOutErr(fmt.Sprintf("artifact ls oci://%s -o json %s", aURL, args))
		g.Expect(err).ToNot(HaveOccurred())

		var list []apiv1.ArtifactReference
		g.Expect(json.Unmarshal([]byte(output), &list)).To(Succeed())

		tags := make([]string, 0, len(list))
		for _, ref := range list {
			tags = append(tags, ref.Tag)
		}
		return tags
	}

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

	t.Run("filters tags by regex", func(t *testing.T) {
		g := NewWithT(t)
		tags := listTags(t, `--filter-regex '^1\.'`)

		g.Expect(tags).To(ConsistOf("1.0.0", "1.1.0"))
	})

	t.Run("filters tags by semver", func(t *testing.T) {
		g := NewWithT(t)
		tags := listTags(t, `--filter-semver '>=1.1.0'`)

		g.Expect(tags).To(ConsistOf("1.1.0", "2.0.0"))
	})

	t.Run("filters tags by regex and semver", func(t *testing.T) {
		g := NewWithT(t)
		tags := listTags(t, `--filter-regex '^1\.' --filter-semver '>=1.1.0'`)

		g.Expect(tags).To(ConsistOf("1.1.0"))
	})

	t.Run("fails for invalid regex filter", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand(fmt.Sprintf("artifact ls oci://%s --filter-regex '['", aURL))
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("invalid regex filter"))
	})

	t.Run("fails for invalid semver filter", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand(fmt.Sprintf("artifact ls oci://%s --filter-semver 'junk'", aURL))
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("invalid semver filter"))
	})

	t.Run("limits the tags", func(t *testing.T) {
		g := NewWithT(t)
		stdout, stderr, err := executeCommandWithOutErr(fmt.Sprintf(
			"artifact ls oci://%s --limit 2 -o json",
			aURL,
		))
		g.Expect(err).ToNot(HaveOccurred())

		var list []apiv1.ArtifactReference
		g.Expect(json.Unmarshal([]byte(stdout), &list)).To(Succeed())
		tags := make([]string, 0, len(list))
		for _, ref := range list {
			tags = append(tags, ref.Tag)
		}
		g.Expect(tags).To(Equal([]string{"latest", "dev", "2.0.0"}))
		g.Expect(stderr).To(ContainSubstring("showing 2 of 4 tags, use --limit 0 for all"))
	})

	t.Run("limits the filtered tags", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(listTags(t, "--filter-semver '>=1.0.0' --limit 1")).To(Equal([]string{"2.0.0"}))
	})

	t.Run("fails for negative limit", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand("artifact ls oci://registry.example.com/org/app --limit -1")
		g.Expect(err).To(MatchError("--limit must be greater than or equal to 0"))
	})

	t.Run("fails for invalid output format", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand("artifact ls oci://registry.example.com/org/app -o junk")
		g.Expect(err).To(MatchError("unknown --output=junk, can be yaml or json"))
	})
}
