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

package testutils

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/v1/random"
)

func TestNewTestRegistryScopesBlobsPerRepository(t *testing.T) {
	g := NewWithT(t)
	addr := g.SetupTestRegistry()

	img, err := random.Image(1024, 1)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(crane.Push(img, addr+"/repo-a:v1")).To(Succeed())

	layers, err := img.Layers()
	g.Expect(err).ToNot(HaveOccurred())
	digest, err := layers[0].Digest()
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(readLayer(addr + "/repo-a@" + digest.String())).To(Succeed())
	g.Expect(readLayer(addr + "/repo-b@" + digest.String())).ToNot(Succeed())
}

func TestIsolateDockerConfig(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("DOCKER_CONFIG", "/original/docker")
	t.Setenv("DOCKER_AUTH_CONFIG", `{"auths":{}}`)
	t.Setenv("REGISTRY_AUTH_FILE", "/original/auth.json")

	restore, err := IsolateDockerConfig()
	g.Expect(err).ToNot(HaveOccurred())

	dir := os.Getenv("DOCKER_CONFIG")
	g.Expect(dir).ToNot(Equal("/original/docker"))
	g.Expect(os.ReadFile(filepath.Join(dir, "config.json"))).To(BeEquivalentTo("{}"))
	for _, key := range []string{"DOCKER_AUTH_CONFIG", "REGISTRY_AUTH_FILE"} {
		_, set := os.LookupEnv(key)
		g.Expect(set).To(BeFalse(), key)
	}

	restore()

	g.Expect(os.Getenv("DOCKER_CONFIG")).To(Equal("/original/docker"))
	g.Expect(os.Getenv("DOCKER_AUTH_CONFIG")).To(Equal(`{"auths":{}}`))
	g.Expect(os.Getenv("REGISTRY_AUTH_FILE")).To(Equal("/original/auth.json"))
	g.Expect(dir).ToNot(BeADirectory())
}

// readLayer downloads the blob behind the layer reference.
func readLayer(ref string) error {
	layer, err := crane.PullLayer(ref)
	if err != nil {
		return err
	}
	rc, err := layer.Compressed()
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(io.Discard, rc)
	return err
}
