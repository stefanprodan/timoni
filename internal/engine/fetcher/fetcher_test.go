/*
Copyright 2024 Stefan Prodan

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
	"testing"

	. "github.com/stefanprodan/timoni/internal/testutils"
)

func TestNew(t *testing.T) {
	ctx := context.Background()

	t.Run("local", func(t *testing.T) {
		g := NewWithT(t)
		f, err := New(ctx, Options{Source: "file://a/b/c"})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(f).To(BeAssignableToTypeOf(&Local{}))
	})
	t.Run("oci", func(t *testing.T) {
		g := NewWithT(t)
		f, err := New(ctx, Options{Source: "oci://a/b/c"})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(f).To(BeAssignableToTypeOf(&OCI{}))
	})
	t.Run("default local", func(t *testing.T) {
		g := NewWithT(t)
		f, err := New(ctx, Options{DefaultLocal: true})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(f).To(BeAssignableToTypeOf(&Local{}))
	})
	t.Run("error", func(t *testing.T) {
		g := NewWithT(t)
		f, err := New(ctx, Options{})

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("unsupported module source"))
		g.Expect(f).To(BeNil())
	})
}
