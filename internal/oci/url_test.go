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
	"context"
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestValidateTag(t *testing.T) {
	g := NewWithT(t)

	for _, tag := range []string{"1.0.0", "_release", strings.Repeat("a", 128)} {
		g.Expect(ValidateTag(tag)).To(Succeed())
	}

	for _, tag := range []string{"", ".release", "-release", strings.Repeat("a", 129)} {
		g.Expect(ValidateTag(tag)).To(MatchError(ContainSubstring("invalid OCI tag")))
	}
}

// TestResolveDigestURL verifies that a mutable reference is turned into the
// immutable digest form that callers use for every subsequent operation.
func TestResolveDigestURL(t *testing.T) {
	g := NewWithT(t)
	opts := Options(context.Background(), "", false)

	imgURL := fmt.Sprintf("oci://%s/%s", dockerRegistry, rnd("my-module"))
	annotations := map[string]string{apiv1.VersionAnnotation: "1.0.0"}
	digestURL, err := PushModule(imgURL+":1.0.0", "testdata/module/", nil, annotations, opts)
	g.Expect(err).ToNot(HaveOccurred())

	// A tag resolves to the digest it currently points at.
	resolved, err := ResolveDigestURL(imgURL+":1.0.0", opts)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resolved).To(Equal(digestURL))
	g.Expect(resolved).To(ContainSubstring("@sha256:"))

	// A digest is already immutable and is left alone.
	resolved, err = ResolveDigestURL(digestURL, opts)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resolved).To(Equal(digestURL))

	_, err = ResolveDigestURL("not-an-oci-url", opts)
	g.Expect(err).To(HaveOccurred())
}

func TestParseArtifactURLTags(t *testing.T) {
	g := NewWithT(t)

	for _, tag := range []string{"1.0.0", "_release", "release-1", "release..1"} {
		_, err := ParseArtifactURL(fmt.Sprintf("oci://example.com/org/artifact:%s", tag))
		g.Expect(err).ToNot(HaveOccurred())
	}

	for _, tag := range []string{".release", "-release"} {
		_, err := ParseArtifactURL(fmt.Sprintf("oci://example.com/org/artifact:%s", tag))
		g.Expect(err).To(MatchError(ContainSubstring("invalid OCI tag")))
	}

	for _, tag := range []string{"release/1", "release+1"} {
		_, err := ParseArtifactURL(fmt.Sprintf("oci://example.com/org/artifact:%s", tag))
		g.Expect(err).To(HaveOccurred())
	}
}
