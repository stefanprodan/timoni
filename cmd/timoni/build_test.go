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

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	ssautil "github.com/fluxcd/pkg/ssa/utils"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestBuild(t *testing.T) {
	modPath := "testdata/module"

	t.Run("builds module with default values", func(t *testing.T) {
		g := NewWithT(t)
		name := rnd("my-instance", 5)
		namespace := rnd("my-namespace", 5)
		output, err := executeCommand(fmt.Sprintf(
			"build -n %s %s %s -p main -o yaml",
			namespace,
			name,
			modPath,
		))
		g.Expect(err).ToNot(HaveOccurred())

		objects, err := ssautil.ReadObjects(strings.NewReader(output))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(output).To(ContainSubstring("tcp://example.internal"))

		for _, o := range objects {
			g.Expect(o.GetKind()).To(BeEquivalentTo("ConfigMap"))
			g.Expect(o.GetName()).To(ContainSubstring(name))
			g.Expect(o.GetNamespace()).To(ContainSubstring(namespace))
		}
	})

	t.Run("builds module and outputs JSON", func(t *testing.T) {
		g := NewWithT(t)
		name := rnd("my-instance", 5)
		namespace := rnd("my-namespace", 5)
		output, err := executeCommand(fmt.Sprintf(
			"build -n %s %s %s -p main -o json",
			namespace,
			name,
			modPath,
		))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(output).To(ContainSubstring("\"kind\": \"List\""))

		objects, err := ssautil.ReadObjects(strings.NewReader(output))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(len(objects)).To(BeEquivalentTo(2))
	})

	t.Run("builds module with custom values", func(t *testing.T) {
		g := NewWithT(t)
		name := rnd("my-instance", 5)
		namespace := rnd("my-namespace", 5)
		output, err := executeCommand(fmt.Sprintf(
			"build -n %s %s %s -f %s -p main -o yaml",
			namespace,
			name,
			modPath,
			modPath+"-values/example.com.cue",
		))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(output).To(ContainSubstring("tcp://example.com"))

		objects, err := ssautil.ReadObjects(strings.NewReader(output))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(len(objects)).To(BeEquivalentTo(2))
		for _, o := range objects {
			g.Expect(o.GetAnnotations()).To(HaveKeyWithValue("scope", "external"))
		}
	})

	t.Run("builds module with YAML and JSON values", func(t *testing.T) {
		g := NewWithT(t)
		name := rnd("my-instance", 5)
		namespace := rnd("my-namespace", 5)
		output, err := executeCommand(fmt.Sprintf(
			"build -n %s %s %s -f %s -f %s -p main -o yaml",
			namespace,
			name,
			modPath,
			modPath+"-values/example.com.yaml",
			modPath+"-values/example.com.json",
		))
		g.Expect(err).ToNot(HaveOccurred())

		// this domain is specified in the YAML file
		g.Expect(output).To(ContainSubstring("tcp://yaml.example.com"))

		objects, err := ssautil.ReadObjects(strings.NewReader(output))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(len(objects)).To(BeEquivalentTo(2))
		for _, o := range objects {
			g.Expect(o.GetAnnotations()).To(HaveKeyWithValue("scope", "from-json"))
		}
	})

	t.Run("builds module with merged values", func(t *testing.T) {
		g := NewWithT(t)
		name := rnd("my-instance", 5)
		namespace := rnd("my-namespace", 5)
		output, err := executeCommand(fmt.Sprintf(
			"build -n %s %s %s -f %s -f %s -f %s -p main -o yaml",
			namespace,
			name,
			modPath,
			modPath+"-values/example.com.cue",
			modPath+"-values/example.io.cue",
			modPath+"-values/client-only.cue",
		))
		g.Expect(err).ToNot(HaveOccurred())

		objects, err := ssautil.ReadObjects(strings.NewReader(output))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(len(objects)).To(BeEquivalentTo(1))
		g.Expect(objects[0].GetName()).To(BeEquivalentTo(name + "-client"))
		g.Expect(objects[0].GetAnnotations()).To(HaveKeyWithValue("scope", "external"))
		g.Expect(objects[0].GetLabels()).To(HaveKeyWithValue("app.kubernetes.io/team", "test"))

		val, _, err := unstructured.NestedString(objects[0].Object, "data", "server")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(val).To(BeEquivalentTo("tcp://example.io:9090"))
	})

	t.Run("fails to build with syntactically invalid file", func(t *testing.T) {
		g := NewWithT(t)
		name := rnd("my-instance", 5)
		namespace := rnd("my-namespace", 5)
		output, err := executeCommand(fmt.Sprintf(
			"build -n %s %s %s -f %s -p main -o yaml",
			namespace,
			name,
			modPath,
			modPath+"-values/badsyntax.cue",
		))
		g.Expect(output).To(BeEmpty())
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("expected")) // "expected TOKEN: found TOKEN" is the form of syntax errors
	})

	t.Run("fails to build with invalid values", func(t *testing.T) {
		g := NewWithT(t)
		name := rnd("my-instance", 5)
		namespace := rnd("my-namespace", 5)
		output, err := executeCommand(fmt.Sprintf(
			"build -n %s %s %s -f %s -p main -o yaml",
			namespace,
			name,
			modPath,
			modPath+"-values/invalid.cue",
		))
		g.Expect(output).To(BeEmpty())
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("client.enabled"))
	})

	t.Run("fails to build with undefined package", func(t *testing.T) {
		g := NewWithT(t)
		name := rnd("my-instance", 5)
		namespace := rnd("my-namespace", 5)
		output, err := executeCommand(fmt.Sprintf(
			"build -n %s %s %s -p test -o yaml",
			namespace,
			name,
			modPath,
		))
		g.Expect(output).To(BeEmpty())
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("cannot find package"))
	})

	t.Run("fails to build with missing values file", func(t *testing.T) {
		g := NewWithT(t)
		name := rnd("my-instance", 5)
		namespace := rnd("my-namespace", 5)
		output, err := executeCommand(fmt.Sprintf(
			"build -n %s %s %s -f %s -p main -o yaml",
			namespace,
			name,
			modPath,
			modPath+"-values/invalid.unknown-extension",
		))
		g.Expect(output).To(BeEmpty())
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("unknown values file format"))
	})

	t.Run("fails to build with kube version", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv("TIMONI_KUBE_VERSION", "1.19.0")
		name := rnd("my-instance", 5)
		namespace := rnd("my-namespace", 5)
		output, err := executeCommand(fmt.Sprintf(
			"build -n %s %s %s -p main -o yaml",
			namespace,
			name,
			modPath,
		))
		g.Expect(output).To(BeEmpty())
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("invalid value 19"))
	})
}

func TestBuild_WithDigest(t *testing.T) {
	g := NewWithT(t)

	instanceName := "frontend"
	modPath := "testdata/module"
	namespace := rnd("my-namespace", 5)
	modName := rnd("my-mod", 5)
	modURL := fmt.Sprintf("%s/%s", dockerRegistry, modName)
	modVer := "1.0.0"

	// Push the module to registry
	pushOut, err := executeCommand(fmt.Sprintf(
		"mod push %s oci://%s -v %s -o json",
		modPath,
		modURL,
		modVer,
	))
	g.Expect(err).ToNot(HaveOccurred())

	// Parse digest
	var mod struct {
		Digest string `json:"digest"`
	}
	err = json.Unmarshal([]byte(pushOut), &mod)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(mod.Digest).ToNot(BeEmpty())

	t.Run("build succeeds if digest matches", func(t *testing.T) {
		g := NewWithT(t)

		_, err := executeCommand(fmt.Sprintf(
			"build -n %s %s oci://%s -v %s -d %s -p main -o yaml",
			namespace,
			instanceName,
			modURL,
			modVer,
			mod.Digest,
		))
		g.Expect(err).NotTo(HaveOccurred())
	})

	t.Run("build with digest succeeds without version", func(t *testing.T) {
		g := NewWithT(t)

		_, err := executeCommand(fmt.Sprintf(
			"build -n %s %s oci://%s -d %s -p main -o yaml",
			namespace,
			instanceName,
			modURL,
			mod.Digest,
		))
		g.Expect(err).NotTo(HaveOccurred())
	})

	t.Run("build errors out if digest differs", func(t *testing.T) {
		g := NewWithT(t)

		_, err := executeCommand(fmt.Sprintf(
			"build -n %s %s oci://%s -v %s -d %s -p main -o yaml",
			namespace,
			instanceName,
			modURL,
			modVer,
			"sha256:123456", // wrong digest
		))
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("digest mismatch, expected sha256:123456 got %s", mod.Digest)))
	})
}
