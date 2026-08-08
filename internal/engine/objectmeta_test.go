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
	"errors"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func makeObject(apiVersion, kind, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{}}
	if apiVersion != "" {
		obj.SetAPIVersion(apiVersion)
	}
	if kind != "" {
		obj.SetKind(kind)
	}
	if name != "" {
		obj.SetName(name)
	}
	return obj
}

func TestValidateObjectMeta_Valid(t *testing.T) {
	g := NewWithT(t)

	obj := makeObject("v1", "ConfigMap", "test")
	obj.SetNamespace("default")
	obj.SetLabels(map[string]string{"app.kubernetes.io/name": "test"})
	obj.SetAnnotations(map[string]string{"timoni.sh/test": "enabled"})

	g.Expect(validateObjectMeta(obj)).To(BeEmpty())
}

func TestValidateObjectMeta_MissingFields(t *testing.T) {
	g := NewWithT(t)

	errs := validateObjectMeta(makeObject("", "ConfigMap", ""))
	g.Expect(errs).To(HaveLen(1))
	g.Expect(errs[0].Error()).To(ContainSubstring("missing required field(s) apiVersion, metadata.name"))

	errs = validateObjectMeta(makeObject("v1", "", "test"))
	g.Expect(errs).To(HaveLen(1))
	g.Expect(errs[0].Error()).To(ContainSubstring("missing required field(s) kind"))
}

func TestValidateObjectMeta_InvalidName(t *testing.T) {
	g := NewWithT(t)

	errs := validateObjectMeta(makeObject("v1", "ConfigMap", "Test_App"))
	g.Expect(errs).To(HaveLen(1))
	g.Expect(errs[0].Error()).To(ContainSubstring(`metadata.name "Test_App" must match regex`))
}

func TestValidateObjectMeta_RBACName(t *testing.T) {
	g := NewWithT(t)

	obj := makeObject("rbac.authorization.k8s.io/v1", "ClusterRole", "system:controller:test")
	g.Expect(validateObjectMeta(obj)).To(BeEmpty())

	obj = makeObject("v1", "ConfigMap", "system:controller:test")
	g.Expect(validateObjectMeta(obj)).To(HaveLen(1))
}

func TestValidateObjectMeta_InvalidNamespace(t *testing.T) {
	g := NewWithT(t)

	obj := makeObject("v1", "ConfigMap", "test")
	obj.SetNamespace("Default")

	errs := validateObjectMeta(obj)
	g.Expect(errs).To(HaveLen(1))
	g.Expect(errs[0].Error()).To(ContainSubstring(`metadata.namespace "Default"`))
}

func TestValidateObjectMeta_InvalidLabels(t *testing.T) {
	g := NewWithT(t)

	obj := makeObject("v1", "ConfigMap", "test")
	obj.Object["metadata"].(map[string]any)["labels"] = map[string]any{
		"-invalid-key": "test",
		"app":          "Invalid Value!",
		"replicas":     int64(2),
	}

	errs := validateObjectMeta(obj)
	g.Expect(errs).To(HaveLen(3))
	g.Expect(errors.Join(errs...).Error()).To(SatisfyAll(
		ContainSubstring(`metadata.labels["-invalid-key"] key must match regex`),
		ContainSubstring(`metadata.labels["app"] value must match regex`),
		ContainSubstring(`metadata.labels["replicas"] must be a string`),
	))
}

func TestValidateObjectMeta_InvalidAnnotations(t *testing.T) {
	g := NewWithT(t)

	obj := makeObject("v1", "ConfigMap", "test")
	obj.Object["metadata"].(map[string]any)["annotations"] = map[string]any{
		"-invalid-key": "test",
		"large":        strings.Repeat("a", 256*1024),
	}

	errs := validateObjectMeta(obj)
	g.Expect(errs).To(HaveLen(2))
	g.Expect(errors.Join(errs...).Error()).To(SatisfyAll(
		ContainSubstring(`metadata.annotations["-invalid-key"] key must match regex`),
		ContainSubstring("metadata.annotations total size must be at most"),
	))
}
