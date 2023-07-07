package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fluxcd/pkg/ssa"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
appName: string @timoni(env:string:TEST_BBUILD_NAME)
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
			values: domain: string @timoni(env:string:TEST_BBUILD_HOST)
		}
		backend: {
			module: {
				url:     "oci://%[1]s"
				version: "%[2]s"
			}
			namespace: string
			values: client: enabled: bool @timoni(env:bool:TEST_BBUILD_ENABLED)
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
					"bundle build -f %s -f %s -f %s -p main",
					cuePath, yamlPath, jsonPath,
				))
			},
			"using stdin": func() (string, error) {
				r := strings.NewReader(bundleData)
				return executeCommandWithIn("bundle build -f - -p main", r)
			},
		}

		for name, execCommand := range execCommands {
			t.Run(name, func(t *testing.T) {
				g := NewWithT(t)
				output, err := execCommand()
				g.Expect(err).ToNot(HaveOccurred())

				objects, err := ssa.ReadObjects(strings.NewReader(output))
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

func getObjectByName(objs []*unstructured.Unstructured, name string) (*unstructured.Unstructured, error) {
	for _, obj := range objs {
		if obj.GetName() == name {
			return obj, nil
		}
	}
	return nil, fmt.Errorf("object with name '%s' does not exist", name)
}
