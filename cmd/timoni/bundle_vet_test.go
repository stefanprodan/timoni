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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
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
	bundleJson := `
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
	instances: {
		podinfo: {
			module: {
				url:     "oci://ghcr.io/stefanprodan/modules/podinfo"
				version: "latest"
			}
			namespace: "podinfo"
			values: {
				caching: {
					enabled:  true
					redisURL: "tcp://:password@test.host:6379"
				}
				monitoring: {
					enabled: true
				}
				autoscaling: {
					enabled: true
				}
			}
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
	g.Expect(os.WriteFile(jsonPath, []byte(bundleJson), 0644)).ToNot(HaveOccurred())

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
	instances: {
		frontend: {
			module: {
				url:     "oci://ghcr.io/stefanprodan/timoni/minimal"
				version: "latest"
			}
			namespace: "fleet-test"
			values: {
				message:  "Hello from cluster staging"
				replicas: 2
				test: {
					enabled: true
				}
			}
		}
	}
}
"production": bundle: {
	apiVersion: "v1alpha1"
	name:       "fleet-test"
	instances: {
		frontend: {
			module: {
				url:     "oci://ghcr.io/stefanprodan/timoni/minimal"
				version: "latest"
			}
			namespace: "fleet-test"
			values: {
				message:  "Hello from cluster production"
				replicas: 3
				test: {
					enabled: true
				}
			}
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
