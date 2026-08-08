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

package engine

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestGetResources(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	steps, err := ExtractValueFromFile(ctx, "testdata/api/apply-steps.cue", apiv1.ApplySelector.String())
	g.Expect(err).ToNot(HaveOccurred())

	sets, err := GetResources(steps)
	g.Expect(err).ToNot(HaveOccurred())

	expectedNames := []string{"app", "addons", "tests"}
	for s, set := range sets {
		g.Expect(sets[s].Name).To(BeEquivalentTo(expectedNames[s]))
		g.Expect(len(set.Objects)).To(BeEquivalentTo(2))
	}
}

func TestGetResources_BytesValue(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	value := ctx.CompileString(`app: [{
		apiVersion: "v1"
		kind:       "Secret"
		metadata: name: "test"
		stringData: key: '\x68\x69'
	}]`)
	g.Expect(value.Err()).ToNot(HaveOccurred())

	sets, err := GetResources(value)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(sets[0].Objects).To(HaveLen(1))

	data, found, err := unstructured.NestedStringMap(sets[0].Objects[0].Object, "stringData")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(data["key"]).To(BeEquivalentTo("hi"))
}

func TestGetResources_NullItem(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	value := ctx.CompileString(`app: [null, {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: "test"
	}]`)
	g.Expect(value.Err()).ToNot(HaveOccurred())

	sets, err := GetResources(value)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(sets[0].Objects).To(HaveLen(1))
	g.Expect(sets[0].Objects[0].GetName()).To(BeEquivalentTo("test"))
}

func TestGetResources_IntegralFloat(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	value := ctx.CompileString(`app: [{
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: "test"
		replicas: 1.0
	}]`)
	g.Expect(value.Err()).ToNot(HaveOccurred())

	sets, err := GetResources(value)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(sets[0].Objects).To(HaveLen(1))
	g.Expect(sets[0].Objects[0].Object["replicas"]).To(BeEquivalentTo(int64(1)))
}

func TestGetResources_EmptyList(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	value := ctx.CompileString(`app: []`)
	g.Expect(value.Err()).ToNot(HaveOccurred())

	sets, err := GetResources(value)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(sets[0].Objects).ToNot(BeNil())
	g.Expect(sets[0].Objects).To(BeEmpty())
}

func TestGetResources_MissingKind(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	value := ctx.CompileString(`app: [{foo: "bar"}]`)
	g.Expect(value.Err()).ToNot(HaveOccurred())

	_, err := GetResources(value)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("Kind"))
}

func TestGetResources_MissingAPIVersion(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	value := ctx.CompileString(`app: [{
		kind: "ConfigMap"
		metadata: name: "test"
	}]`)
	g.Expect(value.Err()).ToNot(HaveOccurred())

	_, err := GetResources(value)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(SatisfyAll(
		ContainSubstring("invalid object at path app[0]"),
		ContainSubstring("missing required field(s) apiVersion"),
	))
}

func TestGetResources_MissingName(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	value := ctx.CompileString(`app: [{
		apiVersion: "v1"
		kind:       "ConfigMap"
	}]`)
	g.Expect(value.Err()).ToNot(HaveOccurred())

	_, err := GetResources(value)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(SatisfyAll(
		ContainSubstring("invalid object at path app[0]"),
		ContainSubstring("missing required field(s) metadata.name"),
	))
}

func TestGetResources_MissingNameBytesValue(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	value := ctx.CompileString(`app: [{
		apiVersion: "v1"
		kind:       "Secret"
		stringData: key: '\x68\x69'
	}]`)
	g.Expect(value.Err()).ToNot(HaveOccurred())

	_, err := GetResources(value)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("missing required field(s) metadata.name"))
}

func TestGetResources_InvalidMetadata(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	value := ctx.CompileString(`app: [{
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {
			name:      "Test_App"
			namespace: "Default"
			labels: app: "Invalid Value!"
		}
	}]`)
	g.Expect(value.Err()).ToNot(HaveOccurred())

	_, err := GetResources(value)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(SatisfyAll(
		ContainSubstring(`metadata.name "Test_App"`),
		ContainSubstring(`metadata.namespace "Default"`),
		ContainSubstring(`metadata.labels["app"] value`),
	))
}

func TestGetResources_AggregatesErrors(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	value := ctx.CompileString(`
	app: [{
		apiVersion: "v1"
		kind:       "ConfigMap"
	}]
	addons: [{
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: "test"
	}, {
		kind: "ConfigMap"
		metadata: name: "test"
	}]`)
	g.Expect(value.Err()).ToNot(HaveOccurred())

	_, err := GetResources(value)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(SatisfyAll(
		ContainSubstring(`resource list "app"`),
		ContainSubstring("invalid object at path app[0]"),
		ContainSubstring(`resource list "addons"`),
		ContainSubstring("invalid object at path addons[1]"),
	))
}
