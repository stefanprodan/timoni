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
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	godigest "github.com/opencontainers/go-digest"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestModuleOperations(t *testing.T) {
	g := NewWithT(t)
	tmpDir := t.TempDir()
	ctx := context.Background()

	srcPath := "testdata/module/"
	imgVersion := "1.0.0"
	imgURL := fmt.Sprintf("oci://%s/%s", dockerRegistry, rnd("my-module"))
	imgVersionURL := fmt.Sprintf("%s:%s", imgURL, imgVersion)
	imgIgnore := []string{"timoni.ignore"}
	imgLicense := "org.opencontainers.image.licenses=Apache-2.0"

	annotations, err := ParseAnnotations([]string{imgLicense})
	g.Expect(err).ToNot(HaveOccurred())
	annotations[apiv1.VersionAnnotation] = imgVersion
	AppendGitMetadata(context.Background(), srcPath, annotations)

	opts := Options(ctx, "", false)
	digestURL, err := PushModule(imgVersionURL, srcPath, imgIgnore, annotations, opts)
	g.Expect(err).ToNot(HaveOccurred())

	err = TagArtifact(digestURL, apiv1.LatestVersion, opts)
	g.Expect(err).ToNot(HaveOccurred())

	list, _, err := ListModuleVersions(ctx, imgURL, ListModuleOptions{WithDigest: true}, opts)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(len(list)).To(BeEquivalentTo(2))
	g.Expect(list[0].Version).To(BeEquivalentTo(apiv1.LatestVersion))
	g.Expect(digestURL).To(ContainSubstring(list[0].Digest))
	g.Expect(digestURL).To(ContainSubstring(list[0].Repository))
	g.Expect(list[1].Version).To(BeEquivalentTo(imgVersion))
	g.Expect(digestURL).To(ContainSubstring(list[1].Digest))
	g.Expect(digestURL).To(ContainSubstring(list[1].Repository))

	dstModPath := filepath.Join(tmpDir, "module-root")
	err = PullArtifact(imgURL, dstModPath, apiv1.TimoniModContentType, opts)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(filepath.Join(dstModPath, "timoni.ignore")).ToNot(BeAnExistingFile())
	g.Expect(filepath.Join(dstModPath, "mod.cue")).ToNot(BeAnExistingFile())
	for _, entry := range []string{
		"templates",
		"templates/cm.cue",
		"templates/config.cue",
		"README.md",
		"timoni.cue",
		"values.cue",
	} {
		g.Expect(filepath.Join(dstModPath, entry)).To(Or(BeAnExistingFile(), BeADirectory()))
	}

	dstVendorPath := filepath.Join(tmpDir, "module-vendor")
	err = PullArtifact(imgURL, dstVendorPath, apiv1.TimoniModVendorContentType, opts)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(filepath.Join(dstVendorPath, "timoni.cue")).ToNot(BeAnExistingFile())
	g.Expect(filepath.Join(dstVendorPath, "templates")).ToNot(BeAnExistingFile())
	for _, entry := range []string{
		"cue.mod",
		"cue.mod/module.cue",
	} {
		g.Expect(filepath.Join(dstVendorPath, entry)).To(Or(BeAnExistingFile(), BeADirectory()))
	}

	dstPath := filepath.Join(tmpDir, "artifact")
	cacheDir := t.TempDir()
	modRef, err := PullModule(digestURL, dstPath, cacheDir, opts)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(modRef.Version).To(BeEquivalentTo(imgVersion))
	g.Expect(filepath.Join(dstPath, "timoni.ignore")).ToNot(BeAnExistingFile())
	for _, entry := range []string{
		"cue.mod",
		"cue.mod/module.cue",
		"templates",
		"templates/cm.cue",
		"templates/config.cue",
		"README.md",
		"timoni.cue",
		"values.cue",
	} {
		g.Expect(filepath.Join(dstPath, entry)).To(Or(BeAnExistingFile(), BeADirectory()))
	}
	cachedLayers, err := os.ReadDir(cacheDir)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(len(cachedLayers)).To(BeEquivalentTo(2))

	blockedDestination := filepath.Join(tmpDir, "blocked-artifact")
	err = os.WriteFile(blockedDestination, []byte("blocked"), 0o600)
	g.Expect(err).ToNot(HaveOccurred())
	_, err = PullModule(digestURL, blockedDestination, cacheDir, opts)
	g.Expect(err).To(HaveOccurred())
	for _, layer := range cachedLayers {
		g.Expect(filepath.Join(cacheDir, layer.Name())).To(BeAnExistingFile())
	}

	corruptLayer := filepath.Join(cacheDir, cachedLayers[0].Name())
	err = os.WriteFile(corruptLayer, []byte("invalid layer"), 0o600)
	g.Expect(err).ToNot(HaveOccurred())

	_, err = PullModule(digestURL, filepath.Join(tmpDir, "recovered-artifact"), cacheDir, opts)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(corruptLayer).To(BeAnExistingFile())
	contents, err := os.ReadFile(corruptLayer)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(contents).ToNot(Equal([]byte("invalid layer")))
}

// TestWriteCachedLayer verifies atomic publication by concurrent writers.
func TestWriteCachedLayer(t *testing.T) {
	g := NewWithT(t)
	cachedLayer := filepath.Join(t.TempDir(), "layer.tgz")
	remote, writer := io.Pipe()
	errCh := make(chan error, 1)
	expectedDigest := godigest.FromString("layer")

	go func() {
		errCh <- writeCachedLayer(cachedLayer, remote, expectedDigest)
	}()

	_, err := writer.Write([]byte("layer"))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cachedLayer).ToNot(BeAnExistingFile())
	g.Expect(writeCachedLayer(cachedLayer, strings.NewReader("layer"), expectedDigest)).To(Succeed())
	g.Expect(writer.Close()).To(Succeed())
	g.Expect(<-errCh).To(Succeed())
	contents, err := os.ReadFile(cachedLayer)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(contents).To(Equal([]byte("layer")))
}

func TestListModuleVersions(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	srcPath := "testdata/module/"
	imgURL := fmt.Sprintf("oci://%s/%s", dockerRegistry, rnd("my-module"))
	opts := Options(ctx, "", false)

	const count = 25
	versions := make([]string, 0, count)
	digests := map[string]string{}
	for i := range count {
		v := fmt.Sprintf("1.0.%d", i)
		annotations := map[string]string{apiv1.VersionAnnotation: v}
		digestURL, err := PushModule(fmt.Sprintf("%s:%s", imgURL, v), srcPath, nil, annotations, opts)
		g.Expect(err).ToNot(HaveOccurred())
		versions = append(versions, v)
		digests[v] = digestURL[strings.LastIndex(digestURL, "@")+1:]
	}
	g.Expect(TagArtifact(fmt.Sprintf("%s:%s", imgURL, versions[count-1]), apiv1.LatestVersion, opts)).To(Succeed())

	// newest first
	expected := make([]string, 0, count)
	for i := count - 1; i >= 0; i-- {
		expected = append(expected, versions[i])
	}

	t.Run("lists all versions with digests", func(t *testing.T) {
		g := NewWithT(t)
		list, total, err := ListModuleVersions(ctx, imgURL, ListModuleOptions{WithDigest: true}, opts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(total).To(Equal(count))
		g.Expect(list).To(HaveLen(count + 1))
		g.Expect(list[0].Version).To(Equal(apiv1.LatestVersion))
		g.Expect(list[0].Digest).To(Equal(digests[versions[count-1]]))
		for i, v := range expected {
			g.Expect(list[i+1].Version).To(Equal(v))
			g.Expect(list[i+1].Digest).To(Equal(digests[v]))
			g.Expect(list[i+1].Repository).To(Equal(imgURL))
		}
	})

	t.Run("limits to the newest versions", func(t *testing.T) {
		g := NewWithT(t)
		list, total, err := ListModuleVersions(ctx, imgURL, ListModuleOptions{WithDigest: true, Limit: 10}, opts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(total).To(Equal(count))
		g.Expect(list).To(HaveLen(11))
		g.Expect(list[0].Version).To(Equal(apiv1.LatestVersion))
		for i, v := range expected[:10] {
			g.Expect(list[i+1].Version).To(Equal(v))
			g.Expect(list[i+1].Digest).To(Equal(digests[v]))
		}
	})

	t.Run("limits without digests", func(t *testing.T) {
		g := NewWithT(t)
		list, total, err := ListModuleVersions(ctx, imgURL, ListModuleOptions{Limit: 3}, opts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(total).To(Equal(count))
		g.Expect(list).To(HaveLen(4))
		for _, ref := range list {
			g.Expect(ref.Digest).To(BeEmpty())
		}
	})

	t.Run("ignores non-semver tags", func(t *testing.T) {
		g := NewWithT(t)
		otherURL := fmt.Sprintf("oci://%s/%s", dockerRegistry, rnd("my-module"))
		_, err := PushModule(fmt.Sprintf("%s:%s", otherURL, "dev"), srcPath, nil, map[string]string{}, opts)
		g.Expect(err).ToNot(HaveOccurred())

		list, total, err := ListModuleVersions(ctx, otherURL, ListModuleOptions{WithDigest: true, Limit: 10}, opts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(total).To(BeZero())
		g.Expect(list).To(BeEmpty())
	})

	t.Run("propagates digest errors", func(t *testing.T) {
		g := NewWithT(t)
		cancelled, cancel := context.WithCancel(ctx)
		cancel()

		list, _, err := ListModuleVersions(cancelled, imgURL, ListModuleOptions{WithDigest: true}, opts)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(MatchRegexp(`failed to get digest for '1\.0\.\d+'`))
		g.Expect(list).To(BeNil())
	})
}
