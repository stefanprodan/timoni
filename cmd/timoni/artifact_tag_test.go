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

	"github.com/google/go-containerregistry/pkg/crane"
	. "github.com/onsi/gomega"
)

func Test_TagArtifact(t *testing.T) {
	aPath := "testdata/module-values"

	g := NewWithT(t)
	aURL := fmt.Sprintf("%s/%s", dockerRegistry, rnd("my-artifact", 5))
	aTag := "1.0.0"

	// Push the artifact to registry
	output, err := executeCommand(fmt.Sprintf(
		"artifact push oci://%s -f %s -t %s --content-type=generic",
		aURL,
		aPath,
		aTag,
	))
	g.Expect(err).ToNot(HaveOccurred())

	// Tag the artifact
	output, err = executeCommand(fmt.Sprintf(
		"artifact tag oci://%s:%s -t 2.0 -t 3 -t latest",
		aURL,
		aTag,
	))
	g.Expect(output).To(ContainSubstring("3"))
	g.Expect(output).To(ContainSubstring("2.0"))
	g.Expect(output).To(ContainSubstring("latest"))

	// List the artifacts
	output, err = executeCommand(fmt.Sprintf(
		"artifact list oci://%s",
		aURL,
	))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(ContainSubstring(aTag))
	g.Expect(output).To(ContainSubstring("3"))
	g.Expect(output).To(ContainSubstring("2.0"))
	g.Expect(output).To(ContainSubstring("latest"))

	// Pull the latest artifact from registry
	_, err = crane.Pull(fmt.Sprintf("%s", aURL))
	g.Expect(err).ToNot(HaveOccurred())
}
