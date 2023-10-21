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

func Test_BundleStatus(t *testing.T) {
	g := NewWithT(t)

	bundleName := rnd("my-bundle", 5)
	modPath := "testdata/module"
	namespace := rnd("my-namespace", 5)
	modName := rnd("my-mod", 5)
	modURL := fmt.Sprintf("%s/%s", dockerRegistry, modName)
	modVer := "1.0.0"

	_, err := executeCommand(fmt.Sprintf(
		"mod push %s oci://%s -v %s",
		modPath,
		modURL,
		modVer,
	))
	g.Expect(err).ToNot(HaveOccurred())

	bundleData := fmt.Sprintf(`
bundle: {
	apiVersion: "v1alpha1"
	name: "%[1]s"
	instances: {
		frontend: {
			module: {
				url:     "oci://%[2]s"
				version: "%[3]s"
			}
			namespace: "%[4]s"
			values: server: enabled: false
		}
		backend: {
			module: {
				url:     "oci://%[2]s"
				version: "%[3]s"
			}
			namespace: "%[4]s"
			values: client: enabled: false
		}
	}
}
`, bundleName, modURL, modVer, namespace)

	_, err = executeCommandWithIn("bundle apply -f - -p main --wait", strings.NewReader(bundleData))
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("lists modules", func(t *testing.T) {
		g := NewWithT(t)

		output, err := executeCommand(fmt.Sprintf("bundle status %s", bundleName))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring(fmt.Sprintf("oci://%s:%s", modURL, modVer)))
		g.Expect(output).To(ContainSubstring("digest sha256:"))
	})

	t.Run("lists ready resources", func(t *testing.T) {
		g := NewWithT(t)

		output, err := executeCommand(fmt.Sprintf("bundle status %s", bundleName))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring(fmt.Sprintf("ConfigMap/%s/frontend-client Current", namespace)))
		g.Expect(output).To(ContainSubstring(fmt.Sprintf("ConfigMap/%s/backend-server Current", namespace)))
	})

	t.Run("lists not found resources", func(t *testing.T) {
		g := NewWithT(t)

		cm := &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "v1",
				APIVersion: "ConfigMap",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "backend-server",
				Namespace: namespace,
			},
		}
		err = envTestClient.Delete(context.Background(), cm)
		g.Expect(err).ToNot(HaveOccurred())

		output, err := executeCommand(fmt.Sprintf("bundle status %s", bundleName))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring(fmt.Sprintf("ConfigMap/%s/frontend-client Current", namespace)))
		g.Expect(output).To(ContainSubstring(fmt.Sprintf("ConfigMap/%s/backend-server NotFound", namespace)))
	})

	t.Run("fails for deleted bundle", func(t *testing.T) {
		g := NewWithT(t)

		_, err := executeCommand(fmt.Sprintf("bundle delete %s --wait", bundleName))
		g.Expect(err).ToNot(HaveOccurred())

		_, err = executeCommand(fmt.Sprintf("bundle status %s", bundleName))
		g.Expect(err).To(HaveOccurred())
	})
}

func Test_BundleStatus_Images(t *testing.T) {
	g := NewWithT(t)

	bundleName := rnd("my-bundle", 5)
	modPath := "testdata/module"
	namespace := rnd("my-namespace", 5)
	modName := rnd("my-mod", 5)
	modURL := fmt.Sprintf("%s/%s", dockerRegistry, modName)
	modVer := "1.0.0"

	_, err := executeCommand(fmt.Sprintf(
		"mod push %s oci://%s -v %s",
		modPath,
		modURL,
		modVer,
	))
	g.Expect(err).ToNot(HaveOccurred())

	bundleData := fmt.Sprintf(`
bundle: {
	apiVersion: "v1alpha1"
	name: "%[1]s"
	instances: {
		timoni: {
			module: {
				url:     "oci://%[2]s"
				version: "%[3]s"
			}
			namespace: "%[4]s"
			values: client: image: digest: ""
		}
	}
}
`, bundleName, modURL, modVer, namespace)

	_, err = executeCommandWithIn("bundle apply -f - -p main --wait", strings.NewReader(bundleData))
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("lists images", func(t *testing.T) {
		g := NewWithT(t)

		output, err := executeCommand(fmt.Sprintf("bundle status %s", bundleName))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring("timoni:latest-dev"))
		g.Expect(output).ToNot(ContainSubstring("timoni:latest-dev@sha"))
	})
}
