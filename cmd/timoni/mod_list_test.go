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
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func Test_ListMod(t *testing.T) {
	g := NewWithT(t)
	modPath := "testdata/module"
	modURL := fmt.Sprintf("%s/%s", dockerRegistry, rnd("my-mod", 5))
	modVers := []string{"1.0.0", "2.0.0", "1.1.0-rc.1"}

	for _, v := range modVers {
		_, err := executeCommand(fmt.Sprintf(
			"mod push %s oci://%s -v %s --latest --resolve-symlinks",
			modPath,
			modURL,
			v,
		))
		g.Expect(err).ToNot(HaveOccurred())
	}

	t.Run("prints table", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"mod ls oci://%s",
			modURL,
		))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(output).To(ContainSubstring("VERSION"))
		g.Expect(output).To(ContainSubstring(apiv1.LatestVersion))
		for _, v := range modVers {
			g.Expect(output).To(ContainSubstring(v))
		}
	})

	t.Run("prints JSON", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"mod ls oci://%s -o json",
			modURL,
		))
		g.Expect(err).ToNot(HaveOccurred())

		var list []apiv1.ModuleReference
		g.Expect(json.Unmarshal([]byte(output), &list)).To(Succeed())

		versions := map[string]bool{}
		for _, ref := range list {
			g.Expect(ref.Repository).To(ContainSubstring(modURL))
			g.Expect(ref.Digest).ToNot(BeEmpty())
			versions[ref.Version] = true
		}
		for _, v := range modVers {
			g.Expect(versions).To(HaveKey(v))
		}
	})

	t.Run("prints YAML", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"mod ls oci://%s -o yaml",
			modURL,
		))
		g.Expect(err).ToNot(HaveOccurred())

		var list []apiv1.ModuleReference
		g.Expect(yaml.Unmarshal([]byte(output), &list)).To(Succeed())

		versions := map[string]bool{}
		for _, ref := range list {
			g.Expect(ref.Digest).ToNot(BeEmpty())
			versions[ref.Version] = true
		}
		for _, v := range modVers {
			g.Expect(versions).To(HaveKey(v))
		}
	})

	t.Run("limits the versions", func(t *testing.T) {
		g := NewWithT(t)
		stdout, stderr, err := executeCommandWithOutErr(fmt.Sprintf(
			"mod ls oci://%s --limit 2",
			modURL,
		))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(stdout).To(ContainSubstring(apiv1.LatestVersion))
		g.Expect(stdout).To(ContainSubstring("2.0.0"))
		g.Expect(stdout).To(ContainSubstring("1.1.0-rc.1"))
		g.Expect(stdout).ToNot(ContainSubstring("1.0.0"))
		g.Expect(stdout).ToNot(ContainSubstring("showing"))
		g.Expect(stderr).To(ContainSubstring("showing 2 of 3 versions, use --limit 0 for all"))
	})

	t.Run("keeps the JSON output clean when limited", func(t *testing.T) {
		g := NewWithT(t)
		stdout, stderr, err := executeCommandWithOutErr(fmt.Sprintf(
			"mod ls oci://%s --limit 1 -o json",
			modURL,
		))
		g.Expect(err).ToNot(HaveOccurred())

		var list []apiv1.ModuleReference
		g.Expect(json.Unmarshal([]byte(stdout), &list)).To(Succeed())
		g.Expect(list).To(HaveLen(2))
		g.Expect(list[0].Version).To(Equal(apiv1.LatestVersion))
		g.Expect(list[1].Version).To(Equal("2.0.0"))
		g.Expect(stderr).To(ContainSubstring("showing 1 of 3 versions"))
	})

	t.Run("prints no notice when all versions fit", func(t *testing.T) {
		g := NewWithT(t)
		_, stderr, err := executeCommandWithOutErr(fmt.Sprintf(
			"mod ls oci://%s --limit 0",
			modURL,
		))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(stderr).ToNot(ContainSubstring("showing"))
	})

	t.Run("fails for negative limit", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand("mod ls oci://registry.example.com/org/module --limit -1")
		g.Expect(err).To(MatchError("--limit must be greater than or equal to 0"))
	})

	t.Run("fails for invalid output format", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand("mod ls oci://registry.example.com/org/module -o junk")
		g.Expect(err).To(MatchError("unknown --output=junk, can be yaml or json"))
	})
}
