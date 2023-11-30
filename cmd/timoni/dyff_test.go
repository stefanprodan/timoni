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
	"bytes"
	"os"
	"testing"

	. "github.com/onsi/gomega"
)

func TestDiffYAML(t *testing.T) {
	g := NewWithT(t)

	liveFile, err := os.CreateTemp("", "live")
	g.Expect(err).ToNot(HaveOccurred())
	defer os.Remove(liveFile.Name())

	mergedFile, err := os.CreateTemp("", "merged")
	g.Expect(err).ToNot(HaveOccurred())
	defer os.Remove(mergedFile.Name())

	err = os.WriteFile(liveFile.Name(), []byte("apiVersion: v1\nkind: Pod\nmetadata:\n  name: test-pod\n"), 0644)
	g.Expect(err).ToNot(HaveOccurred())

	err = os.WriteFile(mergedFile.Name(), []byte("apiVersion: v1\nkind: Pod\nmetadata:\n  name: test-pod-merged\n"), 0644)
	g.Expect(err).ToNot(HaveOccurred())

	buf := new(bytes.Buffer)
	err = diffYAML(liveFile.Name(), mergedFile.Name(), buf)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(buf.String()).To(ContainSubstring("name: test-pod-merged"))
}
