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

package mask

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func secret(data map[string]any, stringData map[string]any) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name":      "test",
			"namespace": "default",
		},
	}}
	if data != nil {
		obj.Object["data"] = data
	}
	if stringData != nil {
		obj.Object["stringData"] = stringData
	}
	return obj
}

func TestSecretData(t *testing.T) {
	t.Run("masks data and stringData", func(t *testing.T) {
		g := NewWithT(t)
		obj := secret(
			map[string]any{"password": "cGFzcw=="},
			map[string]any{"token": "plain"},
		)

		masked := SecretData(obj)

		g.Expect(masked.Object["data"]).To(Equal(map[string]any{"password": Value}))
		g.Expect(masked.Object["stringData"]).To(Equal(map[string]any{"token": Value}))
		g.Expect(masked.GetName()).To(Equal("test"))
	})

	t.Run("does not mutate the input", func(t *testing.T) {
		g := NewWithT(t)
		obj := secret(map[string]any{"password": "cGFzcw=="}, nil)

		SecretData(obj)

		g.Expect(obj.Object["data"]).To(Equal(map[string]any{"password": "cGFzcw=="}))
	})

	t.Run("leaves other kinds unchanged", func(t *testing.T) {
		g := NewWithT(t)
		obj := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": "test"},
			"data":       map[string]any{"key": "value"},
		}}

		g.Expect(SecretData(obj)).To(BeIdenticalTo(obj))
		g.Expect(obj.Object["data"]).To(Equal(map[string]any{"key": "value"}))
	})

	t.Run("masks malformed data fields as a whole", func(t *testing.T) {
		g := NewWithT(t)
		obj := secret(nil, nil)
		obj.Object["data"] = "not-a-map"
		obj.Object["stringData"] = []any{"a", "b"}

		masked := SecretData(obj)

		g.Expect(masked.Object["data"]).To(Equal(Value))
		g.Expect(masked.Object["stringData"]).To(Equal(Value))
	})

	t.Run("masks the last-applied annotation", func(t *testing.T) {
		g := NewWithT(t)
		obj := secret(map[string]any{"password": "cGFzcw=="}, nil)
		obj.SetAnnotations(map[string]string{
			"kubectl.kubernetes.io/last-applied-configuration": `{"data":{"password":"cGFzcw=="}}`,
			"app.kubernetes.io/managed-by":                     "timoni",
		})

		masked := SecretData(obj)

		g.Expect(masked.GetAnnotations()).To(HaveKeyWithValue("kubectl.kubernetes.io/last-applied-configuration", Value))
		g.Expect(masked.GetAnnotations()).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "timoni"))
		g.Expect(obj.GetAnnotations()["kubectl.kubernetes.io/last-applied-configuration"]).To(ContainSubstring("cGFzcw=="))
	})

	t.Run("handles nil and empty secrets", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(SecretData(nil)).To(BeNil())
		g.Expect(SecretData(secret(nil, nil)).Object).ToNot(HaveKey("data"))
	})
}
