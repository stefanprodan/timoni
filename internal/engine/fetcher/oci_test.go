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

package fetcher

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stefanprodan/timoni/internal/oci"

	. "github.com/stefanprodan/timoni/internal/testutils"
)

func TestNewOCI(t *testing.T) {
	g := NewWithT(t)
	of := NewOCI(
		context.Background(),
		"src",
		"1.0.0",
		"dst",
		"cache",
		"creds",
		false,
	)

	g.Expect(of).ToNot(BeNil())
	g.Expect(of).To(Implement((*Fetcher)(nil)))
}

func TestOCIGetModuleRoot(t *testing.T) {
	g := NewWithT(t)
	of := NewOCI(context.Background(), "src", "1.0.0", "dst", "cache", "creds", false)

	g.Expect(of.GetModuleRoot()).To(Equal("dst/module"))
}

func TestOCIFetch(t *testing.T) {
	g := NewWithT(t)
	registry := g.SetupTestRegistry()

	srcPath := "testdata/module/"
	imgVersion := "1.0.0"
	imgURL := fmt.Sprintf("oci://%s/%s", registry, "foo")
	imgVersionURL := fmt.Sprintf("%s:%s", imgURL, imgVersion)
	imgIgnore := []string{"timoni.ignore"}
	opts := oci.Options(context.Background(), "", false)
	digestUrl, err := oci.PushModule(imgVersionURL, srcPath, imgIgnore, map[string]string{}, opts)
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("with version", func(t *testing.T) {
		g := NewWithT(t)

		of := NewOCI(
			context.Background(),
			imgURL,
			imgVersion,
			filepath.Join(t.TempDir(), "dst"),
			filepath.Join(t.TempDir(), "cache"),
			"",
			true,
		)

		mr, err := of.Fetch()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(mr.Repository).To(Equal(imgURL))
		g.Expect(filepath.Join(of.GetModuleRoot(), "cue.mod/module.cue")).To(BeARegularFile())
	})

	t.Run("with digest", func(t *testing.T) {
		g := NewWithT(t)

		digest := digestUrl[strings.LastIndex(digestUrl, "@"):]
		of := NewOCI(
			context.Background(),
			imgURL,
			digest,
			filepath.Join(g.TempDir(), "dst"),
			filepath.Join(g.TempDir(), "cache"),
			"",
			true,
		)

		mr, err := of.Fetch()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(mr.Repository).To(Equal(imgURL))
		g.Expect(filepath.Join(of.GetModuleRoot(), "cue.mod/module.cue")).To(BeARegularFile())

	})
}
