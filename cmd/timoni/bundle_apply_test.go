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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"

	. "github.com/onsi/gomega"
)

func Test_BundleApply(t *testing.T) {
	g := NewWithT(t)

	modPath := "testdata/module"
	namespace := rnd("my-namespace", 5)
	modName := rnd("my-mod", 5)
	modURL := fmt.Sprintf("%s/%s", dockerRegistry, modName)
	modVer := "1.0.0"

	// Push the module to registry
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
	instances: {
		frontend: {
			module: {
				url:     "oci://%[1]s"
				version: "%[2]s"
			}
			namespace: "%[3]s"
			values: server: enabled: false
		}
		backend: {
			module: {
				url:     "oci://%[1]s"
				version: "%[2]s"
			}
			namespace: "%[3]s"
			values: client: enabled: false
		}
	}
}
`, modURL, modVer, namespace)

	bundlePath := filepath.Join(t.TempDir(), "bundle.cue")
	err = os.WriteFile(bundlePath, []byte(bundleData), 0644)
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("creates instances from bundle", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"bundle apply -f %s -p main --wait",
			bundlePath,
		))
		g.Expect(err).ToNot(HaveOccurred())
		t.Log("\n", output)

		clientCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-client", "frontend"),
				Namespace: namespace,
			},
		}

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
		g.Expect(err).ToNot(HaveOccurred())

		serverCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-server", "backend"),
				Namespace: namespace,
			},
		}

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(serverCM), serverCM)
		g.Expect(err).ToNot(HaveOccurred())
	})
}
