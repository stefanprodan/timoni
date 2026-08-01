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

package flags

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func TestCredentialsSet(t *testing.T) {
	g := NewWithT(t)

	var inline Credentials
	g.Expect(inline.Set("user:password")).To(Succeed())
	g.Expect(inline.String()).To(Equal("user:password"))

	var escaped Credentials
	g.Expect(escaped.Set("@@user:password")).To(Succeed())
	g.Expect(escaped.String()).To(Equal("@user:password"))

	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	contents := []byte("user:password \r\n")
	g.Expect(os.WriteFile(path, contents, 0o600)).To(Succeed())

	var fromFile Credentials
	g.Expect(fromFile.Set("@" + path)).To(Succeed())
	g.Expect(fromFile.String()).To(Equal("user:password "))
}

func TestCredentialsSetRejectsInvalidFiles(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")

	var credentials Credentials
	err := credentials.Set("@" + missing)
	g.Expect(err).To(MatchError(ContainSubstring("reading credentials file")))
	g.Expect(err.Error()).ToNot(ContainSubstring("password"))

	oversized := filepath.Join(dir, "oversized")
	g.Expect(os.WriteFile(oversized, bytes.Repeat([]byte("x"), 64<<10+1), 0o600)).To(Succeed())
	err = credentials.Set("@" + oversized)
	g.Expect(err).To(MatchError(ContainSubstring("credentials file exceeds 65536 bytes")))
}
