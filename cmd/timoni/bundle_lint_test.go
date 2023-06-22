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

func Test_BundleLint(t *testing.T) {

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
				version: "latest" @timoni(env:strings:TEST_BLINT_VER)
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
				"bundle lint -f %s",
				bundlePath,
			))

			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(MatchRegexp(tt.matchErr))
		})
	}
}
