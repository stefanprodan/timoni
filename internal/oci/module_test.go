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
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestModuleOperations(t *testing.T) {
	g := NewWithT(t)
	tmpDir := t.TempDir()
	ctx := context.Background()

	srcPath := "testdata/module/"
	imgVersion := "1.0.0"
	imgURL := fmt.Sprintf("oci://%s/%s", dockerRegistry, rnd("my-module", 5))
	imgVersionURL := fmt.Sprintf("%s:%s", imgURL, imgVersion)
	imgIgnore := []string{"timoni.ignore"}
	imgLicense := "org.opencontainers.image.licenses=Apache-2.0"

	annotations, err := ParseAnnotations([]string{imgLicense})
	g.Expect(err).ToNot(HaveOccurred())
	annotations[apiv1.VersionAnnotation] = imgVersion
	AppendGitMetadata(srcPath, annotations)

	opts := Options(ctx, "", false)
	digestURL, err := PushModule(imgVersionURL, srcPath, imgIgnore, annotations, opts)
	g.Expect(err).ToNot(HaveOccurred())

	err = TagArtifact(digestURL, apiv1.LatestVersion, opts)
	g.Expect(err).ToNot(HaveOccurred())

	list, err := ListModuleVersions(imgURL, true, opts)
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
}
