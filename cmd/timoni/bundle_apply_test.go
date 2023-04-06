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

	"github.com/google/go-containerregistry/pkg/crane"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		execCommands := map[string]func() (string, error){
			"using a file": func() (string, error) {
				return executeCommand(fmt.Sprintf(
					"bundle apply -f %s -p main --wait",
					bundlePath,
				))
			},
			"using stdin": func() (string, error) {
				r := strings.NewReader(bundleData)
				return executeCommandWithIn("bundle apply -f - -p main --wait", r)
			},
		}

		for name, execCommand := range execCommands {
			t.Run(name, func(t *testing.T) {
				g := NewWithT(t)
				output, err := execCommand()
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
	})
}

func Test_BundleApply_Digest(t *testing.T) {
	g := NewWithT(t)

	modPath := "testdata/module"
	namespace := rnd("my-namespace", 5)
	modName := rnd("my-mod", 5)
	modURL := fmt.Sprintf("%s/%s", dockerRegistry, modName)
	modVer1 := "1.0.0"
	modVer2 := "2.0.0"

	_, err := executeCommand(fmt.Sprintf(
		"mod push %s oci://%s -v %s",
		modPath,
		modURL,
		modVer1,
	))
	g.Expect(err).ToNot(HaveOccurred())

	modDigestv1, err := crane.Digest(fmt.Sprintf("%s:%s", modURL, modVer1))
	g.Expect(err).ToNot(HaveOccurred())

	_, err = executeCommand(fmt.Sprintf(
		"mod push %s oci://%s -v %s --latest",
		modPath,
		modURL,
		modVer2,
	))
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("creates instance for module digest", func(t *testing.T) {
		g := NewWithT(t)

		bundleData := fmt.Sprintf(`
bundle: {
	apiVersion: "v1alpha1"
	instances: {
		test1: {
			module: {
				url:     "oci://%[1]s"
				digest:  "%[3]s"
			}
			namespace: "%[4]s"
		}
	}
}
`, modURL, modVer1, modDigestv1, namespace)

		r := strings.NewReader(bundleData)
		output, err := executeCommandWithIn("bundle apply -f - -p main --wait", r)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring(modVer1))
		g.Expect(output).To(ContainSubstring(modDigestv1))
	})

	t.Run("creates instance for module version digest", func(t *testing.T) {
		g := NewWithT(t)

		bundleData := fmt.Sprintf(`
bundle: {
	apiVersion: "v1alpha1"
	instances: {
		test2: {
			module: {
				url:     "oci://%[1]s"
				version: "%[2]s"
				digest:  "%[3]s"
			}
			namespace: "%[4]s"
		}
	}
}
`, modURL, modVer1, modDigestv1, namespace)

		r := strings.NewReader(bundleData)
		output, err := executeCommandWithIn("bundle apply -f - -p main --wait", r)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring(modVer1))
	})

	t.Run("fails to create instance with digest mismatch", func(t *testing.T) {
		g := NewWithT(t)

		bundleData := fmt.Sprintf(`
bundle: {
	apiVersion: "v1alpha1"
	instances: {
		test3: {
			module: {
				url:     "oci://%[1]s"
				version: "%[2]s"
				digest:  "%[3]s"
			}
			namespace: "%[4]s"
		}
	}
}
`, modURL, modVer2, modDigestv1, namespace)

		r := strings.NewReader(bundleData)
		_, err := executeCommandWithIn("bundle apply -f - -p main --wait", r)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring(modDigestv1))
	})

	t.Run("creates instance for latest module", func(t *testing.T) {
		g := NewWithT(t)

		bundleData := fmt.Sprintf(`
bundle: {
	apiVersion: "v1alpha1"
	instances: {
		test4: {
			module: {
				url:     "oci://%[1]s"
			}
			namespace: "%[2]s"
		}
	}
}
`, modURL, namespace)

		r := strings.NewReader(bundleData)
		output, err := executeCommandWithIn("bundle apply -f - -p main --wait", r)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring(modVer2))
	})
}
