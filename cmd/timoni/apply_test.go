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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestApply(t *testing.T) {
	modPath := "testdata/module"
	tGroup := fmt.Sprintf("%s.%s", strings.ToLower(apiv1.InstanceKind), apiv1.GroupVersion.Group)
	name := rnd("my-instance", 5)
	namespace := rnd("my-namespace", 5)

	t.Run("creates instance with default values", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"apply -n %s %s %s -p main --wait --timeout=10s",
			namespace,
			name,
			modPath,
		))
		g.Expect(err).ToNot(HaveOccurred())
		t.Log("\n", output)

		clientCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-client", name),
				Namespace: namespace,
			},
		}

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(clientCM.GetLabels()).To(HaveKeyWithValue(tGroup+"/name", name))
		g.Expect(clientCM.GetLabels()).To(HaveKeyWithValue(tGroup+"/namespace", namespace))
		g.Expect(clientCM.GetLabels()).To(HaveKeyWithValue("app.kubernetes.io/version", "0.0.0-devel"))
		g.Expect(clientCM.GetLabels()).To(HaveKey("app.kubernetes.io/kube"))
	})

	t.Run("updates instance with custom values", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"apply -n %s %s %s -f %s -p main --wait",
			namespace,
			name,
			modPath,
			modPath+"-values/example.com.cue",
		))
		g.Expect(err).ToNot(HaveOccurred())
		t.Log("\n", output)

		clientCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-client", name),
				Namespace: namespace,
			},
		}

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(clientCM.GetAnnotations()).To(HaveKeyWithValue("scope", "external"))
		g.Expect(clientCM.Data["server"]).To(ContainSubstring("tcp://example.com"))
	})

	t.Run("updates instance with values from stdin", func(t *testing.T) {
		g := NewWithT(t)

		r := strings.NewReader(`values: domain: "example.org"`)

		output, err := executeCommandWithIn(fmt.Sprintf(
			"apply -n %s %s %s -f - -p main --wait",
			namespace,
			name,
			modPath,
		), r)
		g.Expect(err).ToNot(HaveOccurred())
		t.Log("\n", output)

		clientCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-client", name),
				Namespace: namespace,
			},
		}

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(clientCM.Data["server"]).To(ContainSubstring("tcp://example.org"))
	})

	t.Run("prunes resources removed from instance", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"apply -n %s %s %s -f %s -f %s -f %s -p main --wait",
			namespace,
			name,
			modPath,
			modPath+"-values/example.com.cue",
			modPath+"-values/example.io.cue",
			modPath+"-values/server-only.cue",
		))
		g.Expect(err).ToNot(HaveOccurred())
		t.Log("\n", output)

		clientCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-client", name),
				Namespace: namespace,
			},
		}

		serverCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-server", name),
				Namespace: namespace,
			},
		}

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(serverCM), serverCM)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(serverCM.GetAnnotations()).To(HaveKeyWithValue("scope", "external"))
		g.Expect(serverCM.Data["hostname"]).To(BeEquivalentTo("example.io"))

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
		g.Expect(err).To(HaveOccurred())
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	t.Run("uninstalls instance", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"delete -n %s %s --wait",
			namespace,
			name,
		))
		g.Expect(err).ToNot(HaveOccurred())
		t.Log("\n", output)

		serverCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-server", name),
				Namespace: namespace,
			},
		}

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(serverCM), serverCM)
		g.Expect(err).To(HaveOccurred())
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
}

func TestApply_WithBundleConflicts(t *testing.T) {
	g := NewWithT(t)

	bundleName := "my-bundle"
	instanceName := "frontend"
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
	}
}
`, bundleName, modURL, modVer, namespace)

	bundlePath := filepath.Join(t.TempDir(), "bundle.cue")
	err = os.WriteFile(bundlePath, []byte(bundleData), 0644)
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("fails to create instance overlapping with existing bundle-owned instance", func(t *testing.T) {
		g := NewWithT(t)

		_, err = executeCommandWithIn("bundle apply -f - -p main --wait", strings.NewReader(bundleData))
		g.Expect(err).ToNot(HaveOccurred())

		_, err := executeCommand(fmt.Sprintf(
			"apply -n %s %s %s -p main --wait",
			namespace,
			instanceName,
			modPath,
		))
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("instance \"%s\" exists and is managed by bundle \"%s\"", instanceName, bundleName)))

		output, err := executeCommand(fmt.Sprintf("ls -n %[1]s", namespace))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring(bundleName))
	})

	t.Run("create instance overriding existing bundle-owned instance", func(t *testing.T) {
		g := NewWithT(t)

		_, err = executeCommandWithIn("bundle apply -f - -p main --wait", strings.NewReader(bundleData))
		g.Expect(err).ToNot(HaveOccurred())

		_, err := executeCommand(fmt.Sprintf(
			"apply -n %s %s %s -p main --wait --overwrite-ownership",
			namespace,
			instanceName,
			modPath,
		))
		g.Expect(err).ToNot(HaveOccurred())

		output, err := executeCommand(fmt.Sprintf("ls -n %[1]s", namespace))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).ToNot(ContainSubstring(bundleName))
	})

}

func TestApply_Actions(t *testing.T) {
	modPath := "testdata/module"
	name := rnd("my-instance", 5)
	namespace := rnd("my-namespace", 5)

	t.Run("sets prune and force annotation", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand(fmt.Sprintf(
			"apply -n %s %s %s -f %s -f %s -p main --wait",
			namespace,
			name,
			modPath,
			modPath+"-values/skip-prune.cue",
			modPath+"-values/force-apply.cue",
		))
		g.Expect(err).ToNot(HaveOccurred())

		clientCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-client", name),
				Namespace: namespace,
			},
		}

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(clientCM.GetAnnotations()).To(HaveKeyWithValue(apiv1.PruneAction, apiv1.DisabledValue))
		g.Expect(clientCM.GetAnnotations()).To(HaveKeyWithValue(apiv1.ForceAction, apiv1.EnabledValue))
	})

	t.Run("skips pruning resources removed from instance", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand(fmt.Sprintf(
			"apply -n %s %s %s -f %s -f %s -p main --wait",
			namespace,
			name,
			modPath,
			modPath+"-values/skip-prune.cue",
			modPath+"-values/server-only.cue",
		))
		g.Expect(err).ToNot(HaveOccurred())

		clientCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-client", name),
				Namespace: namespace,
			},
		}

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
		g.Expect(err).ToNot(HaveOccurred())
	})
}

func TestApply_GlobalResources(t *testing.T) {
	modPath := "testdata/module"
	name := rnd("my-instance", 5)
	namespace := rnd("my-namespace", 5)
	nsObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-ns", name),
		},
	}

	t.Run("creates instance with global objects", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommandWithIn(fmt.Sprintf(
			"apply -n %s %s %s -f- -p main --wait --timeout=10s",
			namespace,
			name,
			modPath,
		), strings.NewReader("values: ns: enabled: true"))
		g.Expect(err).ToNot(HaveOccurred())
		t.Log("\n", output)

		ns := nsObj.DeepCopy()
		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(ns), ns)
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("uninstalls instance", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"delete -n %s %s --wait=false",
			namespace,
			name,
		))
		g.Expect(err).ToNot(HaveOccurred())
		t.Log("\n", output)
	})
}
