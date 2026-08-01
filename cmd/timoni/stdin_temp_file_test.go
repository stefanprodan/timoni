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
	"errors"
	"os"
	"testing"
	"testing/iotest"

	. "github.com/onsi/gomega"
)

func TestSaveReaderToFileRemovesPartialFile(t *testing.T) {
	g := NewWithT(t)
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)

	_, err := saveReaderToFile(iotest.ErrReader(errors.New("read failed")))
	g.Expect(err).To(MatchError(ContainSubstring("read failed")))

	entries, err := os.ReadDir(tempDir)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(entries).To(BeEmpty())
}
