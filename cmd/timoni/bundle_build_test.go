package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ssautil "github.com/fluxcd/pkg/ssa/utils"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/stefanprodan/timoni/internal/engine"
)

func Test_BundleBuild(t *testing.T) {
	g := NewWithT(t)

	bundleName := "my-bundle"
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

	bundleCue := fmt.Sprintf(`
appName: string @timoni(runtime:string:TEST_BBUILD_NAME)
bundle: {
	apiVersion: "v1alpha1"
	name: string
	instances: {
		"\(appName)": {
			module: {
				url:     "oci://%[1]s"
				version: "%[2]s"
			}
			namespace: "%[3]s"
			values: server: enabled: false
			values: domain: string @timoni(runtime:string:TEST_BBUILD_HOST)
		}
		backend: {
			module: {
				url:     "oci://%[1]s"
				version: "%[2]s"
			}
			namespace: string
			values: client: enabled: bool @timoni(runtime:bool:TEST_BBUILD_ENABLED)
		}
	}
}
`, modURL, modVer, namespace)

	bundleData := bundleCue + fmt.Sprintf(`
bundle: name: "%[1]s"
bundle: instances: backend: namespace: "%[2]s"
`, bundleName, namespace)

	bundleJson := fmt.Sprintf(`
{
	"bundle": {
		"name": "%[1]s"
	}
}
`, bundleName)

	bundleYaml := fmt.Sprintf(`
bundle:
  instances:
    backend:
      namespace: %[1]s
`, namespace)

	wd := t.TempDir()
	cuePath := filepath.Join(wd, "bundle.cue")
	g.Expect(os.WriteFile(cuePath, []byte(bundleCue), 0644)).ToNot(HaveOccurred())

	yamlPath := filepath.Join(wd, "bundle.yaml")
	g.Expect(os.WriteFile(yamlPath, []byte(bundleYaml), 0644)).ToNot(HaveOccurred())

	jsonPath := filepath.Join(wd, "bundle.json")
	g.Expect(os.WriteFile(jsonPath, []byte(bundleJson), 0644)).ToNot(HaveOccurred())

	t.Setenv("TEST_BBUILD_NAME", "frontend")
	t.Setenv("TEST_BBUILD_HOST", "my.host")
	t.Setenv("TEST_BBUILD_ENABLED", "false")

	t.Run("builds instances from bundle", func(t *testing.T) {
		execCommands := map[string]func() (string, error){
			"using files": func() (string, error) {
				return executeCommand(fmt.Sprintf(
					"bundle build -f %s -f %s -f %s -p main --runtime-from-env",
					cuePath, yamlPath, jsonPath,
				))
			},
			"using stdin": func() (string, error) {
				r := strings.NewReader(bundleData)
				return executeCommandWithIn("bundle build -f - -p main --runtime-from-env", r)
			},
		}

		for name, execCommand := range execCommands {
			t.Run(name, func(t *testing.T) {
				g := NewWithT(t)
				output, err := execCommand()
				g.Expect(err).ToNot(HaveOccurred())

				objects, err := ssautil.ReadObjects(strings.NewReader(output))
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(objects).To(HaveLen(2))

				frontendClientCm, err := getObjectByName(objects, "frontend-client")
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(frontendClientCm.GetKind()).To(BeEquivalentTo("ConfigMap"))
				g.Expect(frontendClientCm.GetNamespace()).To(ContainSubstring(namespace))

				server, found, err := unstructured.NestedString(frontendClientCm.Object, "data", "server")
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(found).To(BeTrue())
				g.Expect(server).To(ContainSubstring("my.host"))

				backendClientCm, err := getObjectByName(objects, "backend-server")
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(backendClientCm.GetKind()).To(BeEquivalentTo("ConfigMap"))
				g.Expect(backendClientCm.GetNamespace()).To(ContainSubstring(namespace))

				host, found, err := unstructured.NestedString(backendClientCm.Object, "data", "hostname")
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(found).To(BeTrue())
				g.Expect(host).To(ContainSubstring("example.internal"))
			})
		}
	})
}

func Test_BundleBuild_LocalModule(t *testing.T) {
	g := NewWithT(t)

	modPath := "testdata/module"
	namespace := rnd("my-namespace", 5)

	bundleCue := fmt.Sprintf(`
bundle: {
	apiVersion: "v1alpha1"
	name: "my-bundle"
	instances: {
		backend: {
			module: {
				url:     "file://%[1]s"
			}
			namespace: "%[2]s"
			values: client: enabled: true
		}
	}
}
`, modPath, namespace)

	wd := t.TempDir()
	cuePath := filepath.Join(wd, "bundle.cue")
	g.Expect(os.WriteFile(cuePath, []byte(bundleCue), 0644)).ToNot(HaveOccurred())

	err := engine.CopyModule(modPath, filepath.Join(wd, modPath))
	g.Expect(err).ToNot(HaveOccurred())

	output, err := executeCommand(fmt.Sprintf(
		"bundle build -f %s -p main",
		cuePath,
	))
	g.Expect(err).ToNot(HaveOccurred())

	objects, err := ssautil.ReadObjects(strings.NewReader(output))
	g.Expect(err).ToNot(HaveOccurred())

	backendClientCm, err := getObjectByName(objects, "backend-server")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(backendClientCm.GetKind()).To(BeEquivalentTo("ConfigMap"))
	g.Expect(backendClientCm.GetNamespace()).To(ContainSubstring(namespace))

	host, found, err := unstructured.NestedString(backendClientCm.Object, "data", "hostname")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(host).To(ContainSubstring("example.internal"))

}

func Test_BundleBuild_Runtime(t *testing.T) {
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

	t.Run("fails for multiple clusters", func(t *testing.T) {
		g := NewWithT(t)
		_, err = executeCommandWithIn(
			fmt.Sprintf("bundle build -f- -r %s -p main", runtimePath),
			strings.NewReader(bundleData))
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("select a cluster"))
	})

	t.Run("builds for a single cluster", func(t *testing.T) {
		g := NewWithT(t)

		output, err := executeCommandWithIn(
			fmt.Sprintf("bundle build -f- -r %s -p main --runtime-group=production", runtimePath),
			strings.NewReader(bundleData))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).ToNot(ContainSubstring("staging-app"))
		g.Expect(output).To(ContainSubstring("production-app"))
	})
}

func getObjectByName(objs []*unstructured.Unstructured, name string) (*unstructured.Unstructured, error) {
	for _, obj := range objs {
		if obj.GetName() == name {
			return obj, nil
		}
	}
	return nil, fmt.Errorf("object with name '%s' does not exist", name)
}

func Test_BundleBuild_RejectsUppercaseNames(t *testing.T) {
	// Each bundle is valid except for a single uppercase identifier, so the
	// only reason the build can fail is the rejected name.
	tests := map[string]string{
		"uppercase instance name": `
bundle: {
	apiVersion: "v1alpha1"
	name:       "my-bundle"
	instances: {
		"Frontend": {
			module: url: "file://testdata/module"
			namespace: "apps"
			values: team: "test"
		}
	}
}`,
		"uppercase namespace": `
bundle: {
	apiVersion: "v1alpha1"
	name:       "my-bundle"
	instances: {
		"frontend": {
			module: url: "file://testdata/module"
			namespace: "Apps"
			values: team: "test"
		}
	}
}`,
		"uppercase bundle name": `
bundle: {
	apiVersion: "v1alpha1"
	name:       "My-Bundle"
	instances: {
		"frontend": {
			module: url: "file://testdata/module"
			namespace: "apps"
			values: team: "test"
		}
	}
}`,
	}

	for name, bundleCue := range tests {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)

			wd := t.TempDir()
			cuePath := filepath.Join(wd, "bundle.cue")
			g.Expect(os.WriteFile(cuePath, []byte(bundleCue), 0644)).ToNot(HaveOccurred())
			g.Expect(engine.CopyModule("testdata/module", filepath.Join(wd, "testdata/module"))).ToNot(HaveOccurred())

			_, err := executeCommand(fmt.Sprintf("bundle build -f %s -p main", cuePath))
			g.Expect(err).To(HaveOccurred())
		})
	}
}

func Test_BundleBuild_OutputDir(t *testing.T) {
	g := NewWithT(t)

	modPath := "testdata/module"
	namespace := rnd("my-namespace", 5)

	bundleCue := fmt.Sprintf(`
bundle: {
	apiVersion: "v1alpha1"
	name: "my-bundle"
	instances: {
		backend: {
			module: {
				url: "file://%[1]s"
			}
			namespace: "%[2]s"
			values: team: "test"
		}
		frontend: {
			module: {
				url: "file://%[1]s"
			}
			namespace: "%[2]s"
			values: {
				team: "test"
				server: enabled: false
			}
		}
	}
}
`, modPath, namespace)

	wd := t.TempDir()
	cuePath := filepath.Join(wd, "bundle.cue")
	g.Expect(os.WriteFile(cuePath, []byte(bundleCue), 0644)).ToNot(HaveOccurred())
	g.Expect(engine.CopyModule(modPath, filepath.Join(wd, modPath))).ToNot(HaveOccurred())

	outDir := filepath.Join(wd, "out")
	_, err := executeCommand(fmt.Sprintf(
		"bundle build -f %s -p main --output-dir %s",
		cuePath, outDir,
	))
	g.Expect(err).ToNot(HaveOccurred())

	// One directory per instance.
	for _, instance := range []string{"backend", "frontend"} {
		info, err := os.Stat(filepath.Join(outDir, instance))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(info.IsDir()).To(BeTrue())
	}

	// One file per resource, named like 'kustomize build -o <dir>'.
	// The backend instance has both the client and server enabled.
	backendNames, err := manifestFileNames(filepath.Join(outDir, "backend"))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(backendNames).To(ConsistOf(
		"v1_configmap_backend-client.yaml",
		"v1_configmap_backend-server.yaml",
	))

	// The frontend instance has the server disabled.
	frontendNames, err := manifestFileNames(filepath.Join(outDir, "frontend"))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(frontendNames).To(ConsistOf("v1_configmap_frontend-client.yaml"))

	// The written manifest parses and carries the instance namespace.
	data, err := os.ReadFile(filepath.Join(outDir, "backend", "v1_configmap_backend-client.yaml"))
	g.Expect(err).ToNot(HaveOccurred())
	objects, err := ssautil.ReadObjects(strings.NewReader(string(data)))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(objects).To(HaveLen(1))
	g.Expect(objects[0].GetKind()).To(Equal("ConfigMap"))
	g.Expect(objects[0].GetName()).To(Equal("backend-client"))
	g.Expect(objects[0].GetNamespace()).To(ContainSubstring(namespace))
}

func Test_BundleBuild_OutputDir_MultiNamespace(t *testing.T) {
	g := NewWithT(t)

	modPath := "testdata/module"
	namespace := "apps"

	// Enabling globals makes the module emit a cluster-scoped Namespace and a
	// ClusterRole pinned to the 'default' namespace, so the instance spans more
	// than one namespace and the file names get a namespace prefix.
	bundleCue := fmt.Sprintf(`
bundle: {
	apiVersion: "v1alpha1"
	name: "my-bundle"
	instances: {
		backend: {
			module: {
				url: "file://%[1]s"
			}
			namespace: "%[2]s"
			values: {
				team: "test"
				globals: enabled: true
			}
		}
	}
}
`, modPath, namespace)

	wd := t.TempDir()
	cuePath := filepath.Join(wd, "bundle.cue")
	g.Expect(os.WriteFile(cuePath, []byte(bundleCue), 0644)).ToNot(HaveOccurred())
	g.Expect(engine.CopyModule(modPath, filepath.Join(wd, modPath))).ToNot(HaveOccurred())

	outDir := filepath.Join(wd, "out")
	_, err := executeCommand(fmt.Sprintf(
		"bundle build -f %s -p main --output-dir %s",
		cuePath, outDir,
	))
	g.Expect(err).ToNot(HaveOccurred())

	names, err := manifestFileNames(filepath.Join(outDir, "backend"))
	g.Expect(err).ToNot(HaveOccurred())

	// Namespaced resources are prefixed with their namespace (the instance
	// namespace for the ConfigMaps, 'default' for the ClusterRole), while the
	// cluster-scoped Namespace stays unprefixed.
	g.Expect(names).To(ConsistOf(
		"apps_v1_configmap_backend-client.yaml",
		"apps_v1_configmap_backend-server.yaml",
		"default_rbac.authorization.k8s.io_v1_clusterrole_backend-readonly.yaml",
		"v1_namespace_backend-ns.yaml",
	))
}

func Test_resourceFileName(t *testing.T) {
	newObj := func(apiVersion, kind, namespace, name string) *unstructured.Unstructured {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(apiVersion)
		obj.SetKind(kind)
		if namespace != "" {
			obj.SetNamespace(namespace)
		}
		obj.SetName(name)
		return obj
	}

	tests := []struct {
		name          string
		obj           *unstructured.Unstructured
		withNamespace bool
		want          string
	}{
		{
			name: "core group omits the empty group and lowercases",
			obj:  newObj("v1", "ConfigMap", "apps", "My-CM"),
			want: "v1_configmap_my-cm.yaml",
		},
		{
			name: "named group",
			obj:  newObj("apps/v1", "Deployment", "apps", "web"),
			want: "apps_v1_deployment_web.yaml",
		},
		{
			name: "dotted group is preserved",
			obj:  newObj("rbac.authorization.k8s.io/v1", "ClusterRole", "", "admin"),
			want: "rbac.authorization.k8s.io_v1_clusterrole_admin.yaml",
		},
		{
			name:          "namespace prefix is added when requested",
			obj:           newObj("v1", "Service", "prod", "api"),
			withNamespace: true,
			want:          "prod_v1_service_api.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(resourceFileName(tt.obj, tt.withNamespace)).To(Equal(tt.want))
		})
	}
}

func manifestFileNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names, nil
}
