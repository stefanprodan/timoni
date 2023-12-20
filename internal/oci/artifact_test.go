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
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestArtifactOperations(t *testing.T) {
	g := NewWithT(t)
	tmpDir := t.TempDir()
	ctx := context.Background()

	srcPath := "testdata/module/"
	imgVersion := "0.0.1"
	imgURL := fmt.Sprintf("oci://%s/%s", dockerRegistry, rnd("my-artifact", 5))
	imgVersionURL := fmt.Sprintf("%s:%s", imgURL, imgVersion)
	imgIgnore := []string{"timoni.ignore"}
	imgContentType := "generic"
	imgLicense := "org.opencontainers.image.licenses=Apache-2.0"

	annotations, err := ParseAnnotations([]string{imgLicense})
	g.Expect(err).ToNot(HaveOccurred())
	AppendGitMetadata(srcPath, annotations)

	opts := Options(ctx, "", false)
	digestURL, err := PushArtifact(imgVersionURL, srcPath, imgIgnore, imgContentType, annotations, opts)
	g.Expect(err).ToNot(HaveOccurred())

	err = TagArtifact(digestURL, apiv1.LatestVersion, opts)
	g.Expect(err).ToNot(HaveOccurred())

	list, err := ListArtifactTags(imgURL, true, opts)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(len(list)).To(BeEquivalentTo(2))
	g.Expect(list[0].Tag).To(BeEquivalentTo(apiv1.LatestVersion))
	g.Expect(digestURL).To(ContainSubstring(list[0].Digest))
	g.Expect(digestURL).To(ContainSubstring(list[0].Repository))
	g.Expect(list[1].Tag).To(BeEquivalentTo(imgVersion))
	g.Expect(digestURL).To(ContainSubstring(list[1].Digest))
	g.Expect(digestURL).To(ContainSubstring(list[1].Repository))

	dstPath := filepath.Join(tmpDir, "artifact")
	err = PullArtifact(imgURL, dstPath, imgContentType, opts)
	g.Expect(err).ToNot(HaveOccurred())
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

	err = PullArtifact(digestURL, dstPath, "unknown", opts)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("no layer found"))

	err = PullArtifact(imgVersionURL, dstPath, apiv1.AnyContentType, opts)
	g.Expect(err).ToNot(HaveOccurred())
}
