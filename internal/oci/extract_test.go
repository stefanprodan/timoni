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

package oci

import (
	archiveTar "archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	fluxTar "github.com/fluxcd/pkg/tar"
	. "github.com/onsi/gomega"
)

func TestExtractLayer(t *testing.T) {
	t.Run("skips symlinks", func(t *testing.T) {
		g := NewWithT(t)
		dst := t.TempDir()

		err := extractLayer(tarGzip(t,
			&archiveTar.Header{Name: "link", Linkname: "../outside", Typeflag: archiveTar.TypeSymlink},
			&archiveTar.Header{Name: "file", Mode: 0o600, Size: 1, Typeflag: archiveTar.TypeReg},
		), dst)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(filepath.Join(dst, "link")).ToNot(BeAnExistingFile())
		g.Expect(os.ReadFile(filepath.Join(dst, "file"))).To(Equal([]byte{0}))
	})

	t.Run("limits extracted bytes", func(t *testing.T) {
		g := NewWithT(t)
		err := extractLayer(tarGzip(t, &archiveTar.Header{
			Name: "large", Mode: 0o600, Size: int64(fluxTar.DefaultMaxUntarSize) + 1, Typeflag: archiveTar.TypeReg,
		}), t.TempDir())
		g.Expect(err).To(MatchError(ContainSubstring("bigger than max archive size")))
	})
}

func tarGzip(t *testing.T, headers ...*archiveTar.Header) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := archiveTar.NewWriter(gz)
	for _, header := range headers {
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == archiveTar.TypeReg && header.Size > 0 && header.Size <= fluxTar.DefaultMaxUntarSize {
			if _, err := tw.Write(make([]byte, header.Size)); err != nil {
				t.Fatal(err)
			}
		}
	}
	_ = tw.Close()
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(buf.Bytes())
}
