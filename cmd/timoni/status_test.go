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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInstanceStatus(t *testing.T) {
	g := NewWithT(t)
	modPath := "testdata/module"
	modURL := fmt.Sprintf("oci://%s/%s", dockerRegistry, rnd("my-status", 5))
	modVer := "1.0.0"
	name := rnd("my-instance", 5)
	namespace := rnd("my-namespace", 5)

	// Package the module as an OCI artifact and push it to registry
	_, err := executeCommand(fmt.Sprintf(
		"mod push %s %s -v %s",
		modPath,
		modURL,
		modVer,
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

	t.Run("ready status", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"status -n %s %s",
			namespace,
			name,
		))
		g.Expect(err).ToNot(HaveOccurred())

		// Verify status output contains the expected resources
		g.Expect(output).To(ContainSubstring(fmt.Sprintf("ConfigMap/%s/%s-client Current", namespace, name)))
		g.Expect(output).To(ContainSubstring(fmt.Sprintf("ConfigMap/%s/%s-server Current", namespace, name)))
	})

	t.Run("not found status", func(t *testing.T) {
		g := NewWithT(t)

		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "v1",
				APIVersion: "ConfigMap",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name + "-server",
				Namespace: namespace,
			},
		}
		err = envTestClient.Delete(context.Background(), cm)
		g.Expect(err).ToNot(HaveOccurred())

		output, err := executeCommand(fmt.Sprintf(
			"status -n %s %s",
			namespace,
			name,
		))
		g.Expect(err).ToNot(HaveOccurred())

		// Verify status output contains the expected resources
		g.Expect(output).To(ContainSubstring(fmt.Sprintf("ConfigMap/%s/%s-client Current", namespace, name)))
		g.Expect(output).To(ContainSubstring(fmt.Sprintf("ConfigMap/%s/%s-server NotFound", namespace, name)))
	})
}
