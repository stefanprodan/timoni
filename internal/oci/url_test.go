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
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
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
