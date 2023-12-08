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

func TestModVet(t *testing.T) {
	modPath := "testdata/module"

	t.Run("vets module with default values", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"mod vet %s -p main",
			modPath,
		))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(output).To(ContainSubstring("timoni:latest-dev@sha256:"))
		g.Expect(output).To(ContainSubstring("timoni.sh/test valid"))
	})

	t.Run("fails to vet with undefined package", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand(fmt.Sprintf(
			"mod vet %s -p test",
			modPath,
		))
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("cannot find package"))
	})
}

func TestModVetSetName(t *testing.T) {
	modPath := "testdata/module"

	t.Run("vets module with default values", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"mod vet %s -p main --name my-mod",
			modPath,
		))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(output).To(ContainSubstring("timoni:latest-dev@sha256:"))
		g.Expect(output).To(ContainSubstring("timoni.sh/test valid"))
		g.Expect(output).To(ContainSubstring("my-mod"))
	})

	t.Run("fails to vet with undefined package", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand(fmt.Sprintf(
			"mod vet %s -p test --name my-mod",
			modPath,
		))
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("cannot find package"))
	})
}
