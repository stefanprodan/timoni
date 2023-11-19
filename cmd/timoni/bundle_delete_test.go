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
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_BundleDelete(t *testing.T) {
	g := NewWithT(t)

	bundleName := "my-bundle"
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

	t.Run("deletes instances from bundle", func(t *testing.T) {
		g := NewWithT(t)

		_, err := executeCommandWithIn("bundle apply -f - -p main --wait", strings.NewReader(bundleData))
		g.Expect(err).ToNot(HaveOccurred())

		clientCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "frontend-client",
				Namespace: namespace,
			},
		}

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
		g.Expect(err).ToNot(HaveOccurred())

		output, err := executeCommand(fmt.Sprintf("bundle delete %s --wait", bundleName))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring("frontend"))
		g.Expect(output).To(ContainSubstring("backend"))

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
		g.Expect(errors.IsNotFound(err)).To(BeTrue())
	})
}

func Test_BundleDelete_MultiNamespace(t *testing.T) {
	g := NewWithT(t)

	bundleName := "my-bundle"
	modPath := "testdata/module"
	namespaceFrontend := rnd("my-namespace", 5)
	namespaceBackend := rnd("my-namespace", 5)
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
			namespace: "%[5]s"
			values: client: enabled: false
		}
	}
}
`, bundleName, modURL, modVer, namespaceFrontend, namespaceBackend)

	t.Run("deletes multi-namespace instances from named bundle", func(t *testing.T) {
		g := NewWithT(t)

		_, err := executeCommandWithIn("bundle apply -f - -p main --wait", strings.NewReader(bundleData))
		g.Expect(err).ToNot(HaveOccurred())

		clientCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "frontend-client",
				Namespace: namespaceFrontend,
			},
		}

		serverCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "backend-server",
				Namespace: namespaceBackend,
			},
		}

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
		g.Expect(err).ToNot(HaveOccurred())

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(serverCM), serverCM)
		g.Expect(err).ToNot(HaveOccurred())

		output, err := executeCommand(fmt.Sprintf("bundle delete %s --wait", bundleName))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring("frontend"))
		g.Expect(output).To(ContainSubstring("backend"))

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
		g.Expect(errors.IsNotFound(err)).To(BeTrue())

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(serverCM), serverCM)
		g.Expect(errors.IsNotFound(err)).To(BeTrue())
	})
}

func Test_BundleDelete_Runtime(t *testing.T) {
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
	_cluster: string @timoni(runtime:string:TIMONI_CLUSTER_NAME)

	apiVersion: "v1alpha1"
	name: "%[1]s"
	instances: {
		"\(_cluster)-app": {
			module: {
				url:     "oci://%[2]s"
				version: "%[3]s"
			}
			namespace: "%[4]s"
		}
	}
}
`, bundleName, modURL, modVer, namespace)

	runtimeCue := `
runtime: {
	apiVersion: "v1alpha1"
	name:       "fleet-test"
	clusters: {
		"staging": {
			group:       "staging"
			kubeContext: "envtest"
		}
		"production": {
			group:       "production"
			kubeContext: "envtest"
		}
	}
	values: []
}
`

	runtimePath := filepath.Join(t.TempDir(), "runtime.cue")
	g.Expect(os.WriteFile(runtimePath, []byte(runtimeCue), 0644)).ToNot(HaveOccurred())

	_, err = executeCommandWithIn(
		fmt.Sprintf("bundle apply -f- -r %s -p main --wait", runtimePath),
		strings.NewReader(bundleData))
	g.Expect(err).ToNot(HaveOccurred())

	stagingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "staging-app-server",
			Namespace: namespace,
		},
	}

	productionCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "production-app-server",
			Namespace: namespace,
		},
	}

	err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(stagingCM), stagingCM)
	g.Expect(err).ToNot(HaveOccurred())

	err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(productionCM), productionCM)
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("deletes instances across clusters", func(t *testing.T) {
		g := NewWithT(t)

		output, err := executeCommand(fmt.Sprintf("bundle delete %s -r %s --wait", bundleName, runtimePath))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring("staging-app"))
		g.Expect(output).To(ContainSubstring("production-app"))

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(stagingCM), stagingCM)
		g.Expect(errors.IsNotFound(err)).To(BeTrue())

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(productionCM), productionCM)
		g.Expect(errors.IsNotFound(err)).To(BeTrue())
	})
}
