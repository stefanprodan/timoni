/*
Copyright 2026 Stefan Prodan

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
	"testing"
	"time"

	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getWidgetFor returns a getter for the Widget custom resource
// rendered by the module-crd testdata module for the given API group.
func getWidgetFor(ctx context.Context, group, name, namespace string) func() error {
	return func() error {
		widget := &unstructured.Unstructured{}
		widget.SetAPIVersion(group + "/v1")
		widget.SetKind("Widget")
		widget.SetName(name)
		widget.SetNamespace(namespace)
		return envTestClient.Get(ctx, client.ObjectKeyFromObject(widget), widget)
	}
}

func TestApplyWithCRDs(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	modPath := "testdata/module-crd"
	name := rnd("my-widget")
	namespace := rnd("my-namespace")

	// A unique API group per run guarantees the kind is unknown to
	// any discovery data cached before the apply starts.
	group := fmt.Sprintf("%s.testing.timoni.sh", rnd("apply"))

	valuesPath := filepath.Join(t.TempDir(), "values.cue")
	g.Expect(os.WriteFile(valuesPath,
		fmt.Appendf(nil, "values: group: %q\n", group), 0644)).To(Succeed())

	// The first install must apply the CRD and resolve the Widget
	// kind registered by it in the same run.
	output, err := executeCommand(fmt.Sprintf(
		"apply -n %s %s %s -p main -f %s --wait --timeout=60s",
		namespace,
		name,
		modPath,
		valuesPath,
	))
	g.Expect(err).ToNot(HaveOccurred())
	t.Log("\n", output)

	g.Expect(getWidgetFor(ctx, group, name, namespace)()).To(Succeed())

	// The uninstall must remove the custom resource and its CRD.
	_, err = executeCommand(fmt.Sprintf(
		"delete -n %s %s --wait",
		namespace,
		name,
	))
	g.Expect(err).ToNot(HaveOccurred())

	crd := &unstructured.Unstructured{}
	crd.SetAPIVersion("apiextensions.k8s.io/v1")
	crd.SetKind("CustomResourceDefinition")
	crd.SetName("widgets." + group)
	g.Eventually(func() bool {
		err := envTestClient.Get(ctx, client.ObjectKeyFromObject(crd), crd)
		return apierrors.IsNotFound(err)
	}, 10*time.Second).Should(BeTrue())
}

func Test_BundleApplyWithCRDs(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	bundleName := rnd("my-bundle")
	modPath := "testdata/module-crd"
	namespace := rnd("my-namespace")
	modName := rnd("my-mod")
	modURL := fmt.Sprintf("%s/%s", dockerRegistry, modName)
	modVer := "1.0.0"
	group := fmt.Sprintf("%s.testing.timoni.sh", rnd("bundle"))

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
	name: "%[1]s"
	instances: {
		widgets: {
			module: {
				url:     "oci://%[2]s"
				version: "%[3]s"
			}
			namespace: "%[4]s"
			values: group: "%[5]s"
		}
	}
}
`, bundleName, modURL, modVer, namespace, group)

	bundlePath := filepath.Join(t.TempDir(), "bundle.cue")
	g.Expect(os.WriteFile(bundlePath, []byte(bundleData), 0644)).To(Succeed())

	// The first install must apply the CRD and resolve the Widget
	// kind registered by it in the same run.
	output, err := executeCommand(fmt.Sprintf(
		"bundle apply -f %s -p main --wait --timeout=60s",
		bundlePath,
	))
	g.Expect(err).ToNot(HaveOccurred())
	t.Log("\n", output)

	g.Expect(getWidgetFor(ctx, group, "widgets", namespace)()).To(Succeed())

	// The uninstall must remove the custom resource and its CRD.
	_, err = executeCommand(fmt.Sprintf(
		"bundle delete %s --wait",
		bundleName,
	))
	g.Expect(err).ToNot(HaveOccurred())

	g.Eventually(func() bool {
		return apierrors.IsNotFound(getWidgetFor(ctx, group, "widgets", namespace)())
	}, 10*time.Second).Should(BeTrue())
}
