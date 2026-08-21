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
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stefanprodan/timoni/internal/runtime"
)

// TestApplyTakeoverServicePorts covers the takeover of objects whose
// Helm-managed fields conflict with the module's desired state on
// server-side apply list merging, such as Service ports differing
// only by port number.
func TestApplyTakeoverServicePorts(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	modPath := "testdata/module-svc"
	name := rnd("my-instance")
	namespace := rnd("my-namespace")

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}
	g.Expect(envTestClient.Create(ctx, ns)).To(Succeed())

	// Simulate a Helm v4 release owning the module's Service with
	// ports that differ from the module's desired state.
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"meta.helm.sh/release-name":      "my-release",
				"meta.helm.sh/release-namespace": namespace,
			},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       9898,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt32(9898),
				},
				{
					Name:       "grpc",
					Port:       9999,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt32(9999),
				},
			},
		},
	}
	u, err := runtime.ToUnstructured(svc)
	g.Expect(err).ToNot(HaveOccurred())
	unstructured.RemoveNestedField(u.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(u.Object, "status")
	g.Expect(envTestClient.Apply(ctx, client.ApplyConfigurationFromUnstructured(u),
		client.FieldOwner("helm"), client.ForceOwnership)).To(Succeed())

	output, err := executeCommand(fmt.Sprintf(
		"apply -n %s %s %s -p main --wait",
		namespace,
		name,
		modPath,
	))
	g.Expect(err).ToNot(HaveOccurred())
	t.Log("\n", output)

	result := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	g.Expect(envTestClient.Get(ctx, client.ObjectKeyFromObject(result), result)).To(Succeed())

	// The Helm release ports are replaced with the module's ones.
	g.Expect(result.Spec.Ports).To(HaveLen(1))
	g.Expect(result.Spec.Ports[0].Name).To(Equal("http"))
	g.Expect(result.Spec.Ports[0].Port).To(Equal(int32(80)))

	// The Helm release metadata is removed from the object.
	g.Expect(result.GetAnnotations()).ToNot(HaveKey("meta.helm.sh/release-name"))
	g.Expect(result.GetAnnotations()).ToNot(HaveKey("meta.helm.sh/release-namespace"))

	// The field ownership is transferred from Helm to Timoni.
	g.Expect(result.GetManagedFields()).ToNot(BeEmpty())
	for _, entry := range result.GetManagedFields() {
		g.Expect(entry.Manager).ToNot(Equal("helm"))
	}
}

func TestApplyTakeoverFromHelm(t *testing.T) {
	tests := []struct {
		name   string
		create func(ctx context.Context, cm *corev1.ConfigMap) error
	}{
		{
			// Helm v3 tracks objects under the 'helm'
			// field manager with the Update operation.
			name: "client-side apply",
			create: func(ctx context.Context, cm *corev1.ConfigMap) error {
				return envTestClient.Create(ctx, cm, client.FieldOwner("helm"))
			},
		},
		{
			// Helm v4 tracks objects under the 'helm'
			// field manager with the Apply operation.
			name: "server-side apply",
			create: func(ctx context.Context, cm *corev1.ConfigMap) error {
				u, err := runtime.ToUnstructured(cm)
				if err != nil {
					return err
				}
				unstructured.RemoveNestedField(u.Object, "metadata", "creationTimestamp")
				return envTestClient.Apply(ctx, client.ApplyConfigurationFromUnstructured(u),
					client.FieldOwner("helm"), client.ForceOwnership)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := context.Background()
			modPath := "testdata/module"
			name := rnd("my-instance")
			namespace := rnd("my-namespace")

			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: namespace},
			}
			g.Expect(envTestClient.Create(ctx, ns)).To(Succeed())

			// Simulate a Helm release owning one of the module's objects:
			// the ConfigMap carries the Helm release metadata, its fields
			// are registered under the 'helm' manager, and the 'extra' key
			// is not part of the module's desired state.
			cm := &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-client", name),
					Namespace: namespace,
					Annotations: map[string]string{
						"meta.helm.sh/release-name":      "my-release",
						"meta.helm.sh/release-namespace": namespace,
					},
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "Helm",
					},
				},
				Data: map[string]string{
					"server": "tcp://helm.internal:9090",
					"extra":  "set by helm",
				},
			}
			g.Expect(tt.create(ctx, cm)).To(Succeed())

			output, err := executeCommand(fmt.Sprintf(
				"apply -n %s %s %s -p main --wait",
				namespace,
				name,
				modPath,
			))
			g.Expect(err).ToNot(HaveOccurred())
			t.Log("\n", output)

			result := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-client", name),
					Namespace: namespace,
				},
			}
			g.Expect(envTestClient.Get(ctx, client.ObjectKeyFromObject(result), result)).To(Succeed())

			// The module values take precedence over the Helm release ones.
			g.Expect(result.Data).To(HaveKeyWithValue("server", "tcp://example.internal:9090"))

			// The Helm release metadata is removed from the object.
			g.Expect(result.GetAnnotations()).ToNot(HaveKey("meta.helm.sh/release-name"))
			g.Expect(result.GetAnnotations()).ToNot(HaveKey("meta.helm.sh/release-namespace"))

			// The field ownership is transferred from Helm to Timoni.
			g.Expect(result.GetManagedFields()).ToNot(BeEmpty())
			for _, entry := range result.GetManagedFields() {
				g.Expect(entry.Manager).ToNot(Equal("helm"))
			}
		})
	}
}
