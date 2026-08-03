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

package oci

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	. "github.com/onsi/gomega"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// TestPullModule_DigestMatchesRegistry verifies that the digest reported for a
// module is the one the registry reports for the same reference, which is what
// users pin after looking it up with tools such as the crane CLI.
func TestPullModule_DigestMatchesRegistry(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	opts := Options(ctx, "", false)

	imgURL := fmt.Sprintf("oci://%s/%s", dockerRegistry, rnd("my-module", 5))
	annotations := map[string]string{apiv1.VersionAnnotation: "1.0.0"}
	_, err := PushModule(imgURL+":1.0.0", "testdata/module/", nil, annotations, opts)
	g.Expect(err).ToNot(HaveOccurred())

	tagRef := strings.TrimPrefix(imgURL, apiv1.ArtifactPrefix) + ":1.0.0"
	registryDigest, err := crane.Digest(tagRef, opts...)
	g.Expect(err).ToNot(HaveOccurred())

	modRef, err := PullModule(imgURL+":1.0.0", filepath.Join(t.TempDir(), "module"), "", opts)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(modRef.Digest).To(Equal(registryDigest))
}

// TestPullModule_DigestFromManifest verifies that the digest reported for a
// module is the digest of the manifest that was pulled, and not a value the
// registry reported for the tag. Callers compare it against the digest pinned
// by the user, so a registry that reports one digest while serving another
// manifest must not satisfy the pin.
func TestPullModule_DigestFromManifest(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	opts := Options(ctx, "", false)

	// Publish a module and record the digest it was published under.
	imgURL := fmt.Sprintf("oci://%s/%s", dockerRegistry, rnd("my-module", 5))
	annotations := map[string]string{apiv1.VersionAnnotation: "1.0.0"}
	digestURL, err := PushModule(imgURL+":1.0.0", "testdata/module/", nil, annotations, opts)
	g.Expect(err).ToNot(HaveOccurred())

	_, publishedDigest, found := strings.Cut(digestURL, "@")
	g.Expect(found).To(BeTrue())

	// A registry that reports a digest of its choosing for the tag, while
	// serving the real manifest for the same tag.
	const reportedDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	upstream, err := url.Parse("http://" + dockerRegistry)
	g.Expect(err).ToNot(HaveOccurred())

	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.ModifyResponse = func(resp *http.Response) error {
		if strings.Contains(resp.Request.URL.Path, "/manifests/") &&
			resp.Header.Get("Docker-Content-Digest") != "" {
			resp.Header.Set("Docker-Content-Digest", reportedDigest)
		}
		return nil
	}
	server := httptest.NewServer(proxy)
	defer server.Close()

	proxyHost := strings.TrimPrefix(server.URL, "http://")
	proxyURL := strings.Replace(imgURL, dockerRegistry, proxyHost, 1)

	modRef, err := PullModule(proxyURL+":1.0.0", filepath.Join(t.TempDir(), "module"), "", opts)
	g.Expect(err).ToNot(HaveOccurred())

	// The digest identifies the manifest that was pulled, so a pin against the
	// value the registry reported does not match.
	g.Expect(modRef.Digest).To(Equal(publishedDigest))
	g.Expect(modRef.Digest).ToNot(Equal(reportedDigest))
}
