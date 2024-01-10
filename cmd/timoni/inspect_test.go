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
	"context"
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestInspect(t *testing.T) {
	g := NewWithT(t)
	modPath := "testdata/module"
	modURL := fmt.Sprintf("oci://%s/%s", dockerRegistry, rnd("my-mod", 5))
	modVer := "1.0.0"
	modLicense := "Apache-2.0"
	name := rnd("my-instance", 5)
	namespace := rnd("my-namespace", 5)

	// Package the module as an OCI artifact and push it to registry
	_, err := executeCommand(fmt.Sprintf(
		"mod push %s %s -v %s -a org.opencontainers.image.licenses=%s",
		modPath,
		modURL,
		modVer,
		modLicense,
	))
	g.Expect(err).ToNot(HaveOccurred())

	// Install the module from the registry
	_, err = executeCommandWithIn(fmt.Sprintf(
		"apply -n %s %s %s -v %s -p main --wait -f-",
		namespace,
		name,
		modURL,
		modVer,
	), strings.NewReader(`values: domain: "app.internal"`))
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("inspect module", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"inspect module -n %s %s",
			namespace,
			name,
		))
		g.Expect(err).ToNot(HaveOccurred())

		// Verify inspect output contains the module metadata
		g.Expect(output).To(ContainSubstring(modURL))
		g.Expect(output).To(ContainSubstring(modVer))
		g.Expect(output).To(ContainSubstring(modLicense))
		g.Expect(output).To(ContainSubstring("sha256"))
		g.Expect(output).To(ContainSubstring("timoni.sh/test"))

	})

	t.Run("inspect values", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"inspect values -n %s %s",
			namespace,
			name,
		))
		g.Expect(err).ToNot(HaveOccurred())

		// Verify inspect output contains the user-supplied values and defaults
		g.Expect(output).To(ContainSubstring(`domain: "app.internal"`))
		g.Expect(output).To(ContainSubstring(`team:   "test"`))
	})

	t.Run("inspect resources", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"inspect resources -n %s %s",
			namespace,
			name,
		))
		g.Expect(err).ToNot(HaveOccurred())

		// Verify inspect output contains the expected resources
		g.Expect(output).To(ContainSubstring(fmt.Sprintf("configmap/%s-client", name)))
		g.Expect(output).To(ContainSubstring(fmt.Sprintf("configmap/%s-server", name)))
	})
}

func TestInspect_Latest(t *testing.T) {
	g := NewWithT(t)
	modPath := "testdata/module"
	modURL := fmt.Sprintf("oci://%s/%s", dockerRegistry, rnd("my-mod", 5))
	modVer := "1.0.0"
	name := rnd("my-instance", 5)
	namespace := rnd("my-namespace", 5)

	// Package the module as an OCI artifact and push it to registry
	_, err := executeCommand(fmt.Sprintf(
		"mod push %s %s -v %s --latest",
		modPath,
		modURL,
		modVer,
	))
	g.Expect(err).ToNot(HaveOccurred())

	// Install the latest version from the registry
	_, err = executeCommand(fmt.Sprintf(
		"apply -n %s %s %s -p main --wait",
		namespace,
		name,
		modURL,
	))
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("inspect module", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"inspect module -n %s %s",
			namespace,
			name,
		))
		g.Expect(err).ToNot(HaveOccurred())

		// Verify inspect output contains the module semver
		g.Expect(output).To(ContainSubstring(modVer))
	})
}

func TestInspect_StorageType(t *testing.T) {
	g := NewWithT(t)
	modPath := "testdata/module"
	modURL := fmt.Sprintf("oci://%s/%s", dockerRegistry, rnd("my-mod", 5))
	modVer := "1.0.0"
	name := rnd("my-instance", 5)
	namespace := rnd("my-namespace", 5)

	_, err := executeCommand(fmt.Sprintf(
		"mod push %s %s -v %s --latest",
		modPath,
		modURL,
		modVer,
	))
	g.Expect(err).ToNot(HaveOccurred())

	_, err = executeCommand(fmt.Sprintf(
		"apply -n %s %s %s -p main --wait",
		namespace,
		name,
		modURL,
	))
	g.Expect(err).ToNot(HaveOccurred())

	var secret corev1.Secret
	err = envTestClient.Get(context.Background(), client.ObjectKey{
		Namespace: namespace,
		Name:      fmt.Sprintf("%s.%s", apiv1.FieldManager, name),
	}, &secret)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(secret.Type).To(BeEquivalentTo(apiv1.InstanceStorageType))
	g.Expect(secret.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", name))
	g.Expect(secret.Data).To(HaveKey(strings.ToLower(apiv1.InstanceKind)))
}
