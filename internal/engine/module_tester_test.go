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

package engine

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func newTestObject(apiVersion, kind, namespace, name string) *unstructured.Unstructured {
	object := &unstructured.Unstructured{}
	object.SetAPIVersion(apiVersion)
	object.SetKind(kind)
	object.SetNamespace(namespace)
	object.SetName(name)
	return object
}

func Test_KeyObjects(t *testing.T) {
	g := NewWithT(t)

	keyed, err := keyObjects([]*unstructured.Unstructured{
		newTestObject("v1", "Service", "test", "app"),
		newTestObject("apps/v1", "Deployment", "test", "app"),
		newTestObject("rbac.authorization.k8s.io/v1", "ClusterRole", "", "app"),
		newTestObject("v1", "ConfigMap", "other", "app"),
	})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(keyed).To(HaveKey("Service/test/app"))
	g.Expect(keyed).To(HaveKey("Deployment/test/app"))
	g.Expect(keyed).To(HaveKey("ConfigMap/other/app"))
	// A cluster-scoped object has no namespace segment.
	g.Expect(keyed).To(HaveKey("ClusterRole/app"))
}

// Test_KeyObjectsSameKind checks that objects of the same kind served by
// different API groups stay addressable, rather than one overwriting the other.
func Test_KeyObjectsSameKind(t *testing.T) {
	g := NewWithT(t)

	keyed, err := keyObjects([]*unstructured.Unstructured{
		newTestObject("v1", "Service", "test", "app"),
		newTestObject("serving.knative.dev/v1", "Service", "test", "app"),
		newTestObject("v1", "ConfigMap", "test", "app"),
	})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(keyed).To(HaveLen(3))
	g.Expect(keyed).To(HaveKey("Service.serving.knative.dev/test/app"))
	// An object in the core group has no group to qualify the kind with.
	g.Expect(keyed).To(HaveKey("Service/test/app"))
	// A kind that is not contested keeps the identifier Timoni prints elsewhere.
	g.Expect(keyed).To(HaveKey("ConfigMap/test/app"))
}

// Test_KeyObjectsSameKindOutsideCore checks that both objects carry their API
// group when neither of them is served by the core group.
func Test_KeyObjectsSameKindOutsideCore(t *testing.T) {
	g := NewWithT(t)

	keyed, err := keyObjects([]*unstructured.Unstructured{
		newTestObject("cluster.x-k8s.io/v1beta1", "Cluster", "test", "app"),
		newTestObject("postgresql.cnpg.io/v1", "Cluster", "test", "app"),
	})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(keyed).To(HaveLen(2))
	g.Expect(keyed).To(HaveKey("Cluster.cluster.x-k8s.io/test/app"))
	g.Expect(keyed).To(HaveKey("Cluster.postgresql.cnpg.io/test/app"))
}

// Test_KeyObjectsSameNameDifferentNamespace checks that the namespace is part
// of the identifier, so a module can render the same kind and name twice.
func Test_KeyObjectsSameNameDifferentNamespace(t *testing.T) {
	g := NewWithT(t)

	keyed, err := keyObjects([]*unstructured.Unstructured{
		newTestObject("v1", "ConfigMap", "test", "app"),
		newTestObject("v1", "ConfigMap", "other", "app"),
	})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(keyed).To(HaveLen(2))
	g.Expect(keyed).To(HaveKey("ConfigMap/test/app"))
	g.Expect(keyed).To(HaveKey("ConfigMap/other/app"))
}

// Test_BuildInputsKey checks that the cases sharing a build are exactly the
// ones built with the same inputs, since a case served another case's build
// would be checked against objects it did not ask for.
func Test_BuildInputsKey(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	base := buildInputs{
		name:        "test",
		namespace:   "test",
		kubeVersion: "1.30.0",
		values:      ctx.CompileString(`{replicas: 2}`),
	}

	same := base
	same.values = ctx.CompileString(`{replicas: 2}`)
	g.Expect(same.key()).To(Equal(base.key()))

	for _, tt := range []struct {
		name   string
		mutate func(*buildInputs)
	}{
		{"name", func(in *buildInputs) { in.name = "other" }},
		{"namespace", func(in *buildInputs) { in.namespace = "other" }},
		{"moduleVersion", func(in *buildInputs) { in.moduleVersion = "1.0.0" }},
		{"kubeVersion", func(in *buildInputs) { in.kubeVersion = "1.31.0" }},
		{"values", func(in *buildInputs) { in.values = ctx.CompileString(`{replicas: 3}`) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			other := base
			tt.mutate(&other)
			g.Expect(other.key()).ToNot(Equal(base.key()))
		})
	}
}

// Test_BuildInputsKeyWithoutValues checks that a case declaring no values does
// not share a build with one that declares them.
func Test_BuildInputsKeyWithoutValues(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	defaults := buildInputs{name: "test", namespace: "test"}
	explicit := defaults
	explicit.values = ctx.CompileString(`{replicas: 2}`)

	g.Expect(explicit.key()).ToNot(Equal(defaults.key()))
}

func Test_KeyObjectsIndistinguishable(t *testing.T) {
	g := NewWithT(t)

	_, err := keyObjects([]*unstructured.Unstructured{
		newTestObject("v1", "ConfigMap", "test", "app"),
		newTestObject("v1", "ConfigMap", "test", "app"),
	})
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("more than one object identified by ConfigMap/test/app"))
}
