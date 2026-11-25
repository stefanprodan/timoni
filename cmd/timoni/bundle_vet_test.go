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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_BundleVet(t *testing.T) {

	tests := []struct {
		name     string
		bundle   string
		matchErr string
	}{
		{
			name:     "fails for invalid API Version",
			matchErr: "bundle.apiVersion",
			bundle: `
bundle: {
	apiVersion: "v1alpha2"
	name: "test"
	instances: {
		test: {
			module: {
				url:     "oci://docker.io/test"
				version: "latest"
			}
			namespace: "default"
			values: {}
		}
	}
}
`,
		},
		{
			name:     "fails for invalid module URL",
			matchErr: "bundle.instances.test.module.url",
			bundle: `
bundle: {
	apiVersion: "v1alpha1"
	name: "test"
	instances: {
		test: {
			module: {
				url:     "docker.io/test"
				version: "latest"
			}
			namespace: "default"
			values: {}
		}
	}
}
`,
		},
		{
			name:     "fails for invalid module prop",
			matchErr: "url2",
			bundle: `
bundle: {
	apiVersion: "v1alpha1"
	name: "test"
	instances: {
		test: {
			module: {
				url2:     "oci://docker.io/test"
				version: "latest"
			}
			namespace: "default"
			values: {}
		}
	}
}
`,
		},
		{
			name:     "fails for missing namespace",
			matchErr: "bundle.instances.test.namespace",
			bundle: `
bundle: {
	apiVersion: "v1alpha1"
	name: "test"
	instances: {
		test: {
			module: {
				url:     "oci://docker.io/test"
				version: "latest"
			}
		}
	}
}
`,
		},
		{
			name:     "fails for missing instances",
			matchErr: "no instances",
			bundle: `
bundle: {
	apiVersion: "v1alpha1"
	name: "test"
	instances: {}
}
`,
		},
		{
			name:     "fails for missing name",
			matchErr: "bundle.name",
			bundle: `
bundle: {
	apiVersion: "v1alpha1"
	instances: {
		test: {
			module: {
				url:     "oci://docker.io/test"
				version: "latest"
			}
		}
	}
}
`,
		},
		{
			name:     "fails for invalid attribute",
			matchErr: "unknown type",
			bundle: `
bundle: {
	apiVersion: "v1alpha1"
	name: "test"
	instances: {
		test: {
			namespace: "default"
			module: {
				url:     "oci://docker.io/test"
				version: "latest" @timoni(runtime:strings:TEST_BLINT_VER)
			}
		}
	}
}
`,
		},
		{
			name:     "fails for missing type",
			matchErr: "expected operand",
			bundle: `
bundle: {
	apiVersion: "v1alpha1"
	name: "test"
	instances: {
		test: {
			namespace: "default"
			module: {
				url:      "oci://docker.io/test"
				version!: @timoni(runtime:string:TEST_BLINT_VER)
			}
		}
	}
}
`,
		},
	}

	tmpDir := t.TempDir()
	t.Setenv("TEST_BLINT_VER", "1.0.0")

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			bundlePath := filepath.Join(tmpDir, fmt.Sprintf("bundle-%v.cue", i))
			err := os.WriteFile(bundlePath, []byte(tt.bundle), 0644)
			g.Expect(err).ToNot(HaveOccurred())

			_, err = executeCommand(fmt.Sprintf(
				"bundle vet -f %s --runtime-from-env",
				bundlePath,
			))

			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(MatchRegexp(tt.matchErr))
		})
	}
}

func Test_BundleVet_Offline(t *testing.T) {
	runtimeValueName := strings.ToUpper(strings.ReplaceAll(rnd("bundle-vet"), "-", "_"))
	secretName := rnd("bundle-vet")
	behaviorDir := t.TempDir()

	defaultBundle := `
bundle: {
	apiVersion: "v1alpha1"
	name:       "offline-test"
	instances: app: {
		module: {
			url:     "oci://docker.io/test"
			version: "latest"
		}
		namespace: "default"
		values: {}
	}
}
`
	defaultBundlePath := filepath.Join(behaviorDir, "default-bundle.cue")
	g := NewWithT(t)
	g.Expect(os.WriteFile(defaultBundlePath, []byte(defaultBundle), 0644)).To(Succeed())

	runtimeBundle := fmt.Sprintf(`
bundle: {
	_cluster:      "default" @timoni(runtime:string:TIMONI_CLUSTER_NAME)
	_runtimeValue: *"bundle-default" | string @timoni(runtime:string:%s)

	apiVersion: "v1alpha1"
	name:       "offline-test"
	instances: app: {
		module: {
			url:     "oci://docker.io/test"
			version: "latest"
		}
		namespace: "default"
		values: {
			cluster:      _cluster
			runtimeValue: _runtimeValue
		}
	}
}
`, runtimeValueName)
	runtimeBundlePath := filepath.Join(behaviorDir, "runtime-bundle.cue")
	g.Expect(os.WriteFile(runtimeBundlePath, []byte(runtimeBundle), 0644)).To(Succeed())

	runtimeNoRefs := `
runtime: {
	apiVersion: "v1alpha1"
	name:       "offline-test"
	clusters: "offline-cluster": {
		group:       "testing"
		kubeContext: "missing"
	}
}
`
	runtimeNoRefsPath := filepath.Join(behaviorDir, "runtime-no-refs.cue")
	g.Expect(os.WriteFile(runtimeNoRefsPath, []byte(runtimeNoRefs), 0644)).To(Succeed())

	runtimeWithRefs := fmt.Sprintf(`
runtime: {
	apiVersion: "v1alpha1"
	name:       "offline-test"
	clusters: "envtest": {
		group:       "testing"
		kubeContext: "envtest"
	}
	values: [
		{
			query: "k8s:v1:Secret:kube-system:%s"
			for: {
				%q: "obj.data.value"
			}
		},
	]
}
`, secretName, runtimeValueName)
	runtimeWithRefsPath := filepath.Join(behaviorDir, "runtime-with-refs.cue")
	g.Expect(os.WriteFile(runtimeWithRefsPath, []byte(runtimeWithRefs), 0644)).To(Succeed())

	t.Run("vets the default runtime without a kubeconfig", func(t *testing.T) {
		g := NewWithT(t)
		useMissingKubeconfig(t)

		output, err := executeCommand(fmt.Sprintf("bundle vet -f %s", defaultBundlePath))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring("instance is valid"))
		g.Expect(output).To(ContainSubstring("i:app"))
	})

	t.Run("vets runtime clusters without refs or a kubeconfig", func(t *testing.T) {
		g := NewWithT(t)
		useMissingKubeconfig(t)

		output, err := executeCommand(fmt.Sprintf(
			"bundle vet -f %s -r %s -p main --print-value",
			runtimeBundlePath, runtimeNoRefsPath,
		))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring(`"offline-cluster": bundle`))
		g.Expect(output).To(ContainSubstring("cluster:"))
		g.Expect(output).To(ContainSubstring(`"offline-cluster"`))
	})

	t.Run("requires a kubeconfig for runtime refs", func(t *testing.T) {
		g := NewWithT(t)
		useMissingKubeconfig(t)

		_, err := executeCommand(fmt.Sprintf(
			"bundle vet -f %s -r %s -p main --print-value",
			runtimeBundlePath, runtimeWithRefsPath,
		))
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("loading kubeconfig failed"))
	})

	t.Run("offline injects runtime refs from the environment", func(t *testing.T) {
		g := NewWithT(t)
		useMissingKubeconfig(t)
		t.Setenv(runtimeValueName, "environment-value")

		output, err := executeCommand(fmt.Sprintf(
			"bundle vet -f %s -r %s -p main --offline --print-value",
			runtimeBundlePath, runtimeWithRefsPath,
		))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring(`runtimeValue: "environment-value"`))
	})

	t.Run("offline logs missing runtime refs and uses bundle defaults", func(t *testing.T) {
		g := NewWithT(t)
		useMissingKubeconfig(t)
		_, found := os.LookupEnv(runtimeValueName)
		g.Expect(found).To(BeFalse())

		stdout, stderr, err := executeCommandWithOutErr(fmt.Sprintf(
			"bundle vet -f %s -r %s -p main --offline --print-value",
			runtimeBundlePath, runtimeWithRefsPath,
		))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(stdout).To(ContainSubstring(`runtimeValue: *"bundle-default" | string`))
		expectedLog := fmt.Sprintf("runtime value %s not found in environment, using the bundle default", runtimeValueName)
		g.Expect(stderr).To(ContainSubstring(expectedLog))
	})

	t.Run("cluster values override environment values", func(t *testing.T) {
		g := NewWithT(t)
		t.Setenv(runtimeValueName, "environment-value")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: "kube-system",
			},
			StringData: map[string]string{"value": "cluster-value"},
		}
		g.Expect(envTestClient.Create(context.Background(), secret, &client.CreateOptions{
			FieldManager: "timoni",
		})).To(Succeed())
		t.Cleanup(func() {
			_ = envTestClient.Delete(context.Background(), secret)
		})

		output, err := executeCommand(fmt.Sprintf(
			"bundle vet -f %s -r %s -p main --runtime-from-env --print-value",
			runtimeBundlePath, runtimeWithRefsPath,
		))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring(`runtimeValue: "cluster-value"`))
		g.Expect(output).ToNot(ContainSubstring("environment-value"))

		output, err = executeCommand(fmt.Sprintf(
			"bundle vet -f %s -r %s -p main --offline --print-value",
			runtimeBundlePath, runtimeWithRefsPath,
		))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring(`runtimeValue: "environment-value"`))
		g.Expect(output).ToNot(ContainSubstring("cluster-value"))
	})
}

// useMissingKubeconfig points commands at a nonexistent kubeconfig for the duration of a test.
func useMissingKubeconfig(t *testing.T) {
	t.Helper()
	original := kubeconfigArgs.KubeConfig
	missing := filepath.Join(t.TempDir(), "missing-kubeconfig")
	kubeconfigArgs.KubeConfig = &missing
	t.Cleanup(func() {
		kubeconfigArgs.KubeConfig = original
	})
}

func Test_BundleVet_PrintValue(t *testing.T) {
	g := NewWithT(t)

	bundleCue := `
bundle: {
	apiVersion: "v1alpha1"
	name:       "podinfo"
	_secrets: {
		host:     string @timoni(runtime:string:TEST_BVET_HOST)
		password: string @timoni(runtime:string:TEST_BVET_PASS)
	}
	instances: {
		podinfo: {
			module: url: "oci://ghcr.io/stefanprodan/modules/podinfo"
			module: version: "latest"
			namespace: "podinfo"
			values: caching: {
				enabled:  true
				redisURL: "tcp://:\(_secrets.password)@\(_secrets.host):6379"
			}
		}
	}
}
`
	bundleYaml := `
bundle:
  instances:
    podinfo:
      values:
        monitoring:
          enabled: true
`
	bundleJSON := `
{
  "bundle": {
    "instances": {
      "podinfo": {
        "values": {
          "autoscaling": {
            "enabled": true
          }
        }
      }
    }
  }
}
`
	bundleComputed := `bundle: {
	apiVersion: "v1alpha1"
	name:       "podinfo"
	instances: podinfo: {
		module: {
			url:     "oci://ghcr.io/stefanprodan/modules/podinfo"
			version: "latest"
		}
		namespace: "podinfo"
		values: {
			autoscaling: enabled: true
			caching: {
				enabled:  true
				redisURL: "tcp://:password@test.host:6379"
			}
			monitoring: enabled: true
		}
	}
}
`
	wd := t.TempDir()
	cuePath := filepath.Join(wd, "bundle.cue")
	g.Expect(os.WriteFile(cuePath, []byte(bundleCue), 0644)).ToNot(HaveOccurred())

	yamlPath := filepath.Join(wd, "bundle.yaml")
	g.Expect(os.WriteFile(yamlPath, []byte(bundleYaml), 0644)).ToNot(HaveOccurred())

	jsonPath := filepath.Join(wd, "bundle.json")
	g.Expect(os.WriteFile(jsonPath, []byte(bundleJSON), 0644)).ToNot(HaveOccurred())

	t.Setenv("TEST_BVET_HOST", "test.host")
	t.Setenv("TEST_BVET_PASS", "password")

	output, err := executeCommand(fmt.Sprintf(
		"bundle vet -f %s -f %s -f %s -p main --runtime-from-env --print-value",
		cuePath, yamlPath, jsonPath,
	))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(BeEquivalentTo(bundleComputed))
}

func Test_BundleVet_Clusters(t *testing.T) {
	g := NewWithT(t)

	bundleCue := `
bundle: {
	_cluster: "dev" @timoni(runtime:string:TIMONI_CLUSTER_NAME)
	_env:     "dev" @timoni(runtime:string:TIMONI_CLUSTER_GROUP)

	apiVersion: "v1alpha1"
	name:       "fleet-test"
	instances: {
		"frontend": {
			module: {
				url:     "oci://ghcr.io/stefanprodan/timoni/minimal"
				version: "latest"
			}
			namespace: "fleet-test"
			values: {
				message: "Hello from cluster \(_cluster)"
				test: enabled: true

				if _env == "staging" {
					replicas: 2
				}

				if _env == "production" {
					replicas: 3
				}
			}
		}
	}
}
`
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
	values: [
		{
			query: "k8s:v1:Namespace:kube-system"
			for: {
				"CLUSTER_UID": "obj.metadata.uid"
			}
		},
	]
}
`

	bundleComputed := `"staging": bundle: {
	apiVersion: "v1alpha1"
	name:       "fleet-test"
	instances: frontend: {
		module: {
			url:     "oci://ghcr.io/stefanprodan/timoni/minimal"
			version: "latest"
		}
		namespace: "fleet-test"
		values: {
			message: "Hello from cluster staging"
			test: enabled: true
			replicas: 2
		}
	}
}
"production": bundle: {
	apiVersion: "v1alpha1"
	name:       "fleet-test"
	instances: frontend: {
		module: {
			url:     "oci://ghcr.io/stefanprodan/timoni/minimal"
			version: "latest"
		}
		namespace: "fleet-test"
		values: {
			message: "Hello from cluster production"
			test: enabled: true
			replicas: 3
		}
	}
}
`
	wd := t.TempDir()
	bundlePath := filepath.Join(wd, "bundle.cue")
	g.Expect(os.WriteFile(bundlePath, []byte(bundleCue), 0644)).ToNot(HaveOccurred())

	runtimePath := filepath.Join(wd, "runtime.cue")
	g.Expect(os.WriteFile(runtimePath, []byte(runtimeCue), 0644)).ToNot(HaveOccurred())

	output, err := executeCommand(fmt.Sprintf(
		"bundle vet -f %s -r %s -p main --print-value",
		bundlePath, runtimePath,
	))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(BeEquivalentTo(bundleComputed))
}

func Test_BundleVet_Workdir(t *testing.T) {
	g := NewWithT(t)

	moduleCue := `
module: "test.example/bundles"
language: version: "v0.14.0"
`
	imagesJSON := `{"podinfo": {"digest": "sha256:abc123"}}`

	instanceCue := `
@extern(embed)

package instance

_images: _ @embed(file="images.json")

Podinfo: {
	module: url:     "oci://ghcr.io/stefanprodan/modules/podinfo"
	module: version: "6.7.0"
	namespace: "podinfo"
	values: image: digest: _images.podinfo.digest
}
`
	bundleCue := `
import "test.example/bundles/instance"

bundle: {
	apiVersion: "v1alpha1"
	name:       "podinfo"
	instances: podinfo: instance.Podinfo
}
`
	bundleComputed := `bundle: {
	apiVersion: "v1alpha1"
	name:       "podinfo"
	instances: podinfo: {
		module: {
			url:     "oci://ghcr.io/stefanprodan/modules/podinfo"
			version: "6.7.0"
		}
		namespace: "podinfo"
		values: image: digest: "sha256:abc123"
	}
}
`
	wd := t.TempDir()
	g.Expect(os.MkdirAll(filepath.Join(wd, "cue.mod"), 0755)).ToNot(HaveOccurred())
	g.Expect(os.MkdirAll(filepath.Join(wd, "instance"), 0755)).ToNot(HaveOccurred())
	g.Expect(os.WriteFile(filepath.Join(wd, "cue.mod", "module.cue"), []byte(moduleCue), 0644)).ToNot(HaveOccurred())
	g.Expect(os.WriteFile(filepath.Join(wd, "instance", "instance.cue"), []byte(instanceCue), 0644)).ToNot(HaveOccurred())
	g.Expect(os.WriteFile(filepath.Join(wd, "instance", "images.json"), []byte(imagesJSON), 0644)).ToNot(HaveOccurred())

	bundlePath := filepath.Join(wd, "bundle.cue")
	g.Expect(os.WriteFile(bundlePath, []byte(bundleCue), 0644)).ToNot(HaveOccurred())

	t.Run("resolves imports and embeds from workdir", func(t *testing.T) {
		g := NewWithT(t)

		output, err := executeCommand(fmt.Sprintf(
			"bundle vet -f %s --workdir %s -p main --print-value",
			bundlePath, wd,
		))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(BeEquivalentTo(bundleComputed))
	})

	t.Run("fails without workdir", func(t *testing.T) {
		g := NewWithT(t)

		_, err := executeCommand(fmt.Sprintf(
			"bundle vet -f %s -p main --print-value",
			bundlePath,
		))
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("imports are unavailable"))
	})

	t.Run("fails for workdir not found", func(t *testing.T) {
		g := NewWithT(t)

		_, err := executeCommand(fmt.Sprintf(
			"bundle vet -f %s --workdir %s -p main --print-value",
			bundlePath, filepath.Join(wd, "not-found"),
		))
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("invalid workdir"))
	})
}
