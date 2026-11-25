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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	ssautil "github.com/fluxcd/pkg/ssa/utils"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const testCRDWithRules = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.testing.timoni.sh
spec:
  group: testing.timoni.sh
  names:
    kind: Widget
    listKind: WidgetList
    plural: widgets
    singular: widget
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      subresources:
        status: {}
      schema:
        openAPIV3Schema:
          type: object
          description: A widget.
          x-kubernetes-validations:
            - rule: "!has(self.status) || self.status.ready || self.spec.replicas > 0"
              message: "a widget that is not ready needs replicas"
            - rule: "self.metadata.name != 'forbidden'"
              message: "the widget name is forbidden"
          properties:
            spec:
              type: object
              description: The widget spec.
              required: [replicas]
              properties:
                replicas:
                  type: integer
                  description: Number of replicas.
                minReplicas:
                  type: integer
                mode:
                  type: string
                  enum: [auto, "on", "off"]
                  default: auto
                description:
                  type: string
                labels:
                  type: object
                  default:
                    description: widget
                  additionalProperties:
                    type: string
                ports:
                  type: array
                  x-kubernetes-list-type: map
                  x-kubernetes-list-map-keys: [name]
                  items:
                    type: object
                    required: [name]
                    properties:
                      name:
                        type: string
                      port:
                        type: integer
                    x-kubernetes-validations:
                      - rule: "self.port > 0"
                        message: "port must be positive"
                tags:
                  type: array
                  x-kubernetes-list-type: set
                  items:
                    type: string
              x-kubernetes-validations:
                - rule: "!has(self.minReplicas) || self.minReplicas <= self.replicas"
                  message: "minReplicas must not exceed replicas"
                - rule: "self.replicas == oldSelf.replicas"
                  message: "replicas is immutable"
                - rule: "!oldSelf.hasValue() || self.replicas >= oldSelf.value().replicas"
                  message: "replicas must not decrease"
                  optionalOldSelf: true
                - rule: "self.mode != 'off' || self.replicas == 0"
                  message: "mode off requires zero replicas"
                - rule: "!has(self.description) || self.description != 'same'"
                  message: "duplicate message"
                - rule: "!has(self.tags) || !('same' in self.tags)"
                  message: "duplicate message"
            status:
              type: object
              properties:
                ready:
                  type: boolean
              x-kubernetes-validations:
                - rule: "self.ready"
                  message: "status must be ready"
    - name: v2
      served: true
      storage: false
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                replicas:
                  type: integer
`

const testCRDWithoutRules = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.testing.timoni.sh
spec:
  group: testing.timoni.sh
  names:
    kind: Widget
    listKind: WidgetList
    plural: widgets
    singular: widget
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                replicas:
                  type: integer
`

const testCRDWithPruning = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.testing.timoni.sh
spec:
  group: testing.timoni.sh
  names:
    kind: Widget
    listKind: WidgetList
    plural: widgets
    singular: widget
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                items:
                  type: array
                  items:
                    type: object
                    properties:
                      name:
                        type: string
              x-kubernetes-validations:
                - rule: "self.items.all(i, i == self.items[0])"
                  message: "items must be identical"
    - name: v2
      served: true
      storage: false
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              anyOf:
                - x-kubernetes-validations:
                    - rule: "true"
`

const testCRDWithIntOrString = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.testing.timoni.sh
spec:
  group: testing.timoni.sh
  names:
    kind: Widget
    listKind: WidgetList
    plural: widgets
    singular: widget
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                port:
                  anyOf:
                    - type: integer
                    - type: string
                  x-kubernetes-int-or-string: true
                quota:
                  anyOf:
                    - type: integer
                    - type: string
                  pattern: ^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$
                  x-kubernetes-int-or-string: true
              x-kubernetes-validations:
                - rule: "!has(self.port) || type(self.port) == int || self.port == 'http'"
                  message: "port must be a number or http"
                - rule: "has(self.quota) && quantity(string(self.quota)).isGreaterThan(quantity('1Gi'))"
                  message: "quota must exceed 1Gi"
`

func readTestCRD(t *testing.T, data string) *unstructured.Unstructured {
	t.Helper()
	objects, err := ssautil.ReadObjects(strings.NewReader(data))
	if err != nil {
		t.Fatalf("reading CRD failed: %v", err)
	}
	if len(objects) != 1 {
		t.Fatalf("expected one object, got %d", len(objects))
	}
	return objects[0]
}

func newTestWidget(version string, spec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "testing.timoni.sh/" + version,
		"kind":       "Widget",
		"metadata": map[string]any{
			"name":      "test",
			"namespace": "default",
		},
		"spec": spec,
	}}
}

func TestCRDValidator_Validate(t *testing.T) {
	widgetV1 := schema.GroupVersionKind{Group: "testing.timoni.sh", Version: "v1", Kind: "Widget"}
	widgetV2 := schema.GroupVersionKind{Group: "testing.timoni.sh", Version: "v2", Kind: "Widget"}

	newValidator := func(t *testing.T) *CRDValidator {
		g := NewWithT(t)
		v := NewCRDValidator()
		g.Expect(v.AddCRD(readTestCRD(t, testCRDWithRules))).To(Succeed())
		return v
	}

	t.Run("registers schemas per version", func(t *testing.T) {
		g := NewWithT(t)
		v := newValidator(t)

		g.Expect(v.HasSchema(widgetV1)).To(BeTrue())
		g.Expect(v.HasSchema(widgetV2)).To(BeTrue())
		g.Expect(v.Validate(context.Background(), newTestWidget("v2", map[string]any{
			"replicas": int64(0),
		}))).To(BeEmpty())
	})

	t.Run("passes objects that satisfy the schema and the rules", func(t *testing.T) {
		g := NewWithT(t)
		v := newValidator(t)

		errs := v.Validate(context.Background(), newTestWidget("v1", map[string]any{
			"replicas":    int64(3),
			"minReplicas": int64(1),
			"ports": []any{
				map[string]any{"name": "http", "port": int64(8080)},
			},
			"tags": []any{"a", "b"},
		}))
		g.Expect(errs).To(BeEmpty())
	})

	t.Run("reports rule violations with their field path", func(t *testing.T) {
		g := NewWithT(t)
		v := newValidator(t)

		errs := v.Validate(context.Background(), newTestWidget("v1", map[string]any{
			"replicas":    int64(1),
			"minReplicas": int64(5),
			"ports": []any{
				map[string]any{"name": "http", "port": int64(0)},
			},
		}))
		g.Expect(errs).To(HaveLen(2))
		g.Expect(errs[0]).To(MatchError("spec: minReplicas must not exceed replicas"))
		g.Expect(errs[1]).To(MatchError("spec.ports[0]: port must be positive"))
	})

	t.Run("reports root rule violations without a field path", func(t *testing.T) {
		g := NewWithT(t)
		v := newValidator(t)

		object := newTestWidget("v1", map[string]any{"replicas": int64(1)})
		object.SetName("forbidden")
		errs := v.Validate(context.Background(), object)
		g.Expect(errs).To(HaveLen(1))
		g.Expect(errs[0]).To(MatchError("the widget name is forbidden"))
	})

	t.Run("reports every violation sharing a message", func(t *testing.T) {
		g := NewWithT(t)
		v := newValidator(t)

		errs := v.Validate(context.Background(), newTestWidget("v1", map[string]any{
			"replicas":    int64(1),
			"description": "same",
			"tags":        []any{"same"},
		}))
		g.Expect(errs).To(HaveLen(2))
		g.Expect(errs[0]).To(MatchError("spec: duplicate message"))
		g.Expect(errs[1]).To(MatchError("spec: duplicate message"))
	})

	t.Run("validates the OpenAPI schema before the rules", func(t *testing.T) {
		g := NewWithT(t)
		v := newValidator(t)

		errs := v.Validate(context.Background(), newTestWidget("v1", map[string]any{
			"replicas": "many",
			"mode":     "sometimes",
		}))
		g.Expect(errs).To(HaveLen(3))
		g.Expect(errs[0]).To(MatchError(`spec.mode: Unsupported value: "sometimes": supported values: "auto", "on", "off"`))
		g.Expect(errs[1]).To(MatchError(`spec.replicas: Invalid value: "string": must be of type integer: "string"`))
		g.Expect(errs[2].Error()).To(HavePrefix("some validation rules were not checked because the object was invalid"))

		errs = v.Validate(context.Background(), newTestWidget("v1", map[string]any{}))
		g.Expect(errs).To(HaveLen(2))
		g.Expect(errs[0]).To(MatchError("spec.replicas: Required value"))
		g.Expect(errs[1].Error()).To(HavePrefix("some validation rules were not checked"))
	})

	t.Run("validates list maps and sets", func(t *testing.T) {
		g := NewWithT(t)
		v := newValidator(t)

		errs := v.Validate(context.Background(), newTestWidget("v1", map[string]any{
			"replicas": int64(1),
			"ports": []any{
				map[string]any{"name": "http", "port": int64(80)},
				map[string]any{"name": "http", "port": int64(8080)},
			},
			"tags": []any{"a", "a"},
		}))
		g.Expect(errs).To(HaveLen(2))
		g.Expect(errs[0].Error()).To(SatisfyAll(HavePrefix("spec.ports"), ContainSubstring("Duplicate value")))
		g.Expect(errs[1].Error()).To(SatisfyAll(HavePrefix("spec.tags"), ContainSubstring("Duplicate value")))
	})

	t.Run("applies schema defaults before evaluation", func(t *testing.T) {
		g := NewWithT(t)
		v := newValidator(t)

		object := newTestWidget("v1", map[string]any{"replicas": int64(3)})
		g.Expect(v.Validate(context.Background(), object)).To(BeEmpty())
		g.Expect(object.Object["spec"]).ToNot(HaveKey("mode"))

		errs := v.Validate(context.Background(), newTestWidget("v1", map[string]any{
			"replicas": int64(3),
			"mode":     "off",
		}))
		g.Expect(errs).To(HaveLen(1))
		g.Expect(errs[0]).To(MatchError("spec: mode off requires zero replicas"))
	})

	t.Run("prunes and reports unknown fields before evaluation", func(t *testing.T) {
		g := NewWithT(t)
		v := NewCRDValidator()
		g.Expect(v.AddCRD(readTestCRD(t, testCRDWithPruning))).To(Succeed())

		errs := v.Validate(context.Background(), newTestWidget("v1", map[string]any{
			"items": []any{
				map[string]any{"name": "a", "unknown": int64(1)},
				map[string]any{"name": "a"},
			},
		}))
		g.Expect(errs).To(HaveLen(1))
		g.Expect(errs[0]).To(MatchError("spec.items[0].unknown: unknown field"))

		errs = v.Validate(context.Background(), newTestWidget("v1", map[string]any{
			"items": []any{
				map[string]any{"name": "a"},
				map[string]any{"name": "b"},
			},
		}))
		g.Expect(errs).To(HaveLen(1))
		g.Expect(errs[0]).To(MatchError("spec: items must be identical"))
	})

	t.Run("keeps unknown fields when the CRD preserves them", func(t *testing.T) {
		g := NewWithT(t)
		v := NewCRDValidator()
		crd := readTestCRD(t, strings.ReplaceAll(testCRDWithPruning,
			"scope: Namespaced", "scope: Namespaced\n  preserveUnknownFields: true"))
		g.Expect(v.AddCRD(crd)).To(Succeed())

		errs := v.Validate(context.Background(), newTestWidget("v1", map[string]any{
			"items": []any{
				map[string]any{"name": "a", "unknown": int64(1)},
				map[string]any{"name": "a"},
			},
		}))
		g.Expect(errs).To(HaveLen(1))
		g.Expect(errs[0]).To(MatchError("spec: items must be identical"))
	})

	t.Run("sorts the violations by field path", func(t *testing.T) {
		g := NewWithT(t)
		v := newValidator(t)

		object := newTestWidget("v1", map[string]any{
			"replicas":    int64(1),
			"minReplicas": int64(5),
			"description": "same",
			"tags":        []any{"same", "same"},
			"ports": []any{
				map[string]any{"name": "http", "port": int64(0)},
				map[string]any{"name": "http", "port": int64(0)},
			},
			"unknown": "x",
		})
		first := v.Validate(context.Background(), object)
		g.Expect(first).To(HaveLen(8))
		g.Expect(first[0]).To(MatchError("spec.unknown: unknown field"))
		g.Expect(first[1].Error()).To(HavePrefix("spec.ports[1]: Duplicate value"))
		g.Expect(first[2].Error()).To(HavePrefix("spec.tags[1]: Duplicate value"))
		g.Expect(first[3]).To(MatchError("spec: duplicate message"))
		g.Expect(first[4]).To(MatchError("spec: duplicate message"))
		g.Expect(first[5]).To(MatchError("spec: minReplicas must not exceed replicas"))
		g.Expect(first[6]).To(MatchError("spec.ports[0]: port must be positive"))
		g.Expect(first[7]).To(MatchError("spec.ports[1]: port must be positive"))
		for i := 0; i < 20; i++ {
			g.Expect(v.Validate(context.Background(), object)).To(Equal(first))
		}
	})

	t.Run("accepts preserve-unknown-fields false markers", func(t *testing.T) {
		g := NewWithT(t)
		v := NewCRDValidator()
		crd := readTestCRD(t, strings.ReplaceAll(testCRDWithoutRules,
			"type: object\n          properties:", "type: object\n          x-kubernetes-preserve-unknown-fields: false\n          properties:"))
		g.Expect(v.AddCRD(crd)).To(Succeed())
		g.Expect(v.Validate(context.Background(), newTestWidget("v1", map[string]any{"replicas": int64(1)}))).To(BeEmpty())
	})

	t.Run("ignores rules under non-structural keywords", func(t *testing.T) {
		g := NewWithT(t)
		v := NewCRDValidator()
		g.Expect(v.AddCRD(readTestCRD(t, testCRDWithPruning))).To(Succeed())

		g.Expect(v.HasSchema(widgetV2)).To(BeTrue())
		g.Expect(v.Validate(context.Background(), newTestWidget("v2", map[string]any{}))).To(BeEmpty())
	})

	t.Run("evaluates int-or-string fields", func(t *testing.T) {
		g := NewWithT(t)
		v := NewCRDValidator()
		g.Expect(v.AddCRD(readTestCRD(t, testCRDWithIntOrString))).To(Succeed())

		g.Expect(v.Validate(context.Background(), newTestWidget("v1", map[string]any{
			"port":  int64(8080),
			"quota": "2Gi",
		}))).To(BeEmpty())
		g.Expect(v.Validate(context.Background(), newTestWidget("v1", map[string]any{
			"port":  "http",
			"quota": int64(4294967296),
		}))).To(BeEmpty())

		errs := v.Validate(context.Background(), newTestWidget("v1", map[string]any{
			"port":  "https",
			"quota": "512Mi",
		}))
		g.Expect(errs).To(HaveLen(2))
		g.Expect(errs[0]).To(MatchError("spec: port must be a number or http"))
		g.Expect(errs[1]).To(MatchError("spec: quota must exceed 1Gi"))
	})

	t.Run("skips transition rules and evaluates optional ones", func(t *testing.T) {
		g := NewWithT(t)
		v := newValidator(t)

		errs := v.Validate(context.Background(), newTestWidget("v1", map[string]any{
			"replicas": int64(3),
		}))
		g.Expect(errs).To(BeEmpty())
	})

	t.Run("drops the status when the CRD declares the subresource", func(t *testing.T) {
		g := NewWithT(t)
		v := newValidator(t)

		object := newTestWidget("v1", map[string]any{"replicas": int64(0)})
		object.Object["status"] = map[string]any{"ready": false}
		g.Expect(v.Validate(context.Background(), object)).To(BeEmpty())

		v = NewCRDValidator()
		crd := readTestCRD(t, strings.ReplaceAll(testCRDWithRules, "      subresources:\n        status: {}\n", ""))
		g.Expect(v.AddCRD(crd)).To(Succeed())
		errs := v.Validate(context.Background(), object)
		g.Expect(errs).To(HaveLen(2))
		g.Expect(errs[0]).To(MatchError("a widget that is not ready needs replicas"))
		g.Expect(errs[1]).To(MatchError("status: status must be ready"))
	})

	t.Run("ignores kinds without a schema", func(t *testing.T) {
		g := NewWithT(t)
		v := newValidator(t)

		object := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": "test"},
		}}
		g.Expect(v.HasSchema(object.GroupVersionKind())).To(BeFalse())
		g.Expect(v.Validate(context.Background(), object)).To(BeEmpty())
	})

	t.Run("replaces schemas registered for the same version", func(t *testing.T) {
		g := NewWithT(t)
		v := newValidator(t)

		g.Expect(v.AddCRD(readTestCRD(t, testCRDWithoutRules))).To(Succeed())
		g.Expect(v.HasSchema(widgetV1)).To(BeTrue())
		g.Expect(v.HasSchema(widgetV2)).To(BeTrue())
		errs := v.Validate(context.Background(), newTestWidget("v1", map[string]any{
			"replicas": int64(1),
			"mode":     "off",
		}))
		g.Expect(errs).To(HaveLen(1))
		g.Expect(errs[0]).To(MatchError("spec.mode: unknown field"))
	})

	t.Run("reports rule compile errors on a single line", func(t *testing.T) {
		g := NewWithT(t)
		v := NewCRDValidator()
		crd := readTestCRD(t, strings.ReplaceAll(testCRDWithRules,
			`rule: "self.port > 0"`, `rule: "self.undefinedField > 0"`))
		g.Expect(v.AddCRD(crd)).To(Succeed())

		errs := v.Validate(context.Background(), newTestWidget("v1", map[string]any{
			"replicas": int64(3),
			"ports": []any{
				map[string]any{"name": "http", "port": int64(8080)},
			},
		}))
		g.Expect(errs).To(HaveLen(1))
		g.Expect(errs[0].Error()).To(HavePrefix("spec.ports[0]: rule compile error:"))
		g.Expect(errs[0].Error()).ToNot(ContainSubstring("\n"))
	})

	t.Run("reports schemas that cannot be compiled per object", func(t *testing.T) {
		g := NewWithT(t)
		v := NewCRDValidator()
		crd := readTestCRD(t, strings.ReplaceAll(testCRDWithRules,
			"required: [replicas]", "required: replicas"))

		g.Expect(v.AddCRD(crd)).To(Succeed())
		g.Expect(v.HasSchema(widgetV1)).To(BeTrue())
		errs := v.Validate(context.Background(), newTestWidget("v1", map[string]any{}))
		g.Expect(errs).To(HaveLen(1))
		g.Expect(errs[0].Error()).To(HavePrefix("invalid schema in CRD widgets.testing.timoni.sh version v1"))
	})
}

func TestCRDValidator_AddCRDs(t *testing.T) {
	g := NewWithT(t)
	v := NewCRDValidator()

	objects := []*unstructured.Unstructured{
		newTestWidget("v1", map[string]any{"replicas": int64(3)}),
		readTestCRD(t, testCRDWithRules),
	}
	g.Expect(v.AddCRDs(objects)).To(Succeed())
	g.Expect(v.HasSchema(objects[0].GroupVersionKind())).To(BeTrue())
}

func TestCRDValidator_AddVendoredCRDs(t *testing.T) {
	rendered := readTestCRD(t, testCRDWithRules)
	vendored := readTestCRD(t, strings.ReplaceAll(testCRDWithRules,
		"minReplicas must not exceed replicas", "vendored rule"))
	invalid := newTestWidget("v1", map[string]any{
		"replicas":    int64(1),
		"minReplicas": int64(5),
	})

	t.Run("does not replace a rendered schema", func(t *testing.T) {
		g := NewWithT(t)
		v := NewCRDValidator()
		g.Expect(v.AddCRDs([]*unstructured.Unstructured{rendered})).To(Succeed())
		g.Expect(v.AddVendoredCRDs([]*unstructured.Unstructured{vendored})).To(Succeed())

		errs := v.Validate(context.Background(), invalid)
		g.Expect(errs).To(HaveLen(1))
		g.Expect(errs[0].Error()).To(ContainSubstring("minReplicas must not exceed replicas"))
	})

	t.Run("is replaced by a rendered schema", func(t *testing.T) {
		g := NewWithT(t)
		v := NewCRDValidator()
		g.Expect(v.AddVendoredCRDs([]*unstructured.Unstructured{vendored})).To(Succeed())
		g.Expect(v.AddCRDs([]*unstructured.Unstructured{rendered})).To(Succeed())

		errs := v.Validate(context.Background(), invalid)
		g.Expect(errs).To(HaveLen(1))
		g.Expect(errs[0].Error()).To(ContainSubstring("minReplicas must not exceed replicas"))
	})

	t.Run("keeps the first vendored schema", func(t *testing.T) {
		g := NewWithT(t)
		v := NewCRDValidator()
		g.Expect(v.AddVendoredCRDs([]*unstructured.Unstructured{vendored})).To(Succeed())
		g.Expect(v.AddVendoredCRDs([]*unstructured.Unstructured{rendered})).To(Succeed())

		errs := v.Validate(context.Background(), invalid)
		g.Expect(errs).To(HaveLen(1))
		g.Expect(errs[0].Error()).To(ContainSubstring("vendored rule"))
	})
}

func TestCRDValidator_ValidateObjects(t *testing.T) {
	g := NewWithT(t)
	v := NewCRDValidator()
	g.Expect(v.AddCRD(readTestCRD(t, testCRDWithRules))).To(Succeed())

	unknown := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "test", "namespace": "default"},
	}}
	invalid := newTestWidget("v1", map[string]any{
		"replicas":    int64(1),
		"minReplicas": int64(5),
	})

	errs := v.ValidateObjects(context.Background(), []*unstructured.Unstructured{unknown, invalid})
	g.Expect(errs).To(HaveLen(1))
	g.Expect(errs[0]).To(MatchError("Widget/default/test spec: minReplicas must not exceed replicas"))
}

// writeTestModule writes a module importing the given CUE packages,
// each placed under cue.mod at the given relative directory.
func writeTestModule(t *testing.T, packages map[string]string) string {
	t.Helper()
	g := NewWithT(t)
	moduleRoot := t.TempDir()

	g.Expect(os.MkdirAll(filepath.Join(moduleRoot, "cue.mod"), os.ModePerm)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(moduleRoot, "cue.mod", "module.cue"),
		[]byte("module: \"timoni.sh/test\"\nlanguage: version: \"v0.17.1\"\n"), 0644)).To(Succeed())

	var imports, uses strings.Builder
	i := 0
	for dir, content := range packages {
		pkgDir := filepath.Join(moduleRoot, "cue.mod", dir)
		g.Expect(os.MkdirAll(pkgDir, os.ModePerm)).To(Succeed())
		g.Expect(os.WriteFile(filepath.Join(pkgDir, "types_gen.cue"), []byte(content), 0644)).To(Succeed())
		importPath := strings.TrimPrefix(strings.TrimPrefix(dir, "gen/"), "pkg/")
		imports.WriteString(fmt.Sprintf("\tp%d %q\n", i, importPath))
		uses.WriteString(fmt.Sprintf("\tp%d.#Widget,\n", i))
		i++
	}

	main := `package main

import (
` + imports.String() + `)

_imports: [
` + uses.String() + `]

timoni: {
	apiVersion: "v1alpha1"
	instance: {
		config: metadata: {
			name:      string @tag(name)
			namespace: string @tag(namespace)
		}
		objects: {}
	}
	apply: app: []
}
`
	g.Expect(os.WriteFile(filepath.Join(moduleRoot, "timoni.cue"), []byte(main), 0644)).To(Succeed())
	return moduleRoot
}

func TestImporter_EmbeddedCRD(t *testing.T) {
	g := NewWithT(t)
	imp := NewImporter(cuecontext.New(), "// Code generated by timoni. DO NOT EDIT.")

	t.Run("embeds the CRD of every version without documentation", func(t *testing.T) {
		generated, err := imp.Generate([]byte(testCRDWithRules))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(generated).To(HaveKey("testing.timoni.sh/widget/v1"))
		g.Expect(generated).To(HaveKey("testing.timoni.sh/widget/v2"))

		v1 := string(generated["testing.timoni.sh/widget/v1"])
		definitions, embedded, found := strings.Cut(v1, "\n_crd: {")
		g.Expect(found).To(BeTrue())
		g.Expect(definitions).To(ContainSubstring("#WidgetSpec"))
		g.Expect(definitions).ToNot(ContainSubstring("#WidgetStatus"))
		g.Expect(definitions).ToNot(ContainSubstring("status"))
		g.Expect(embedded).To(ContainSubstring("widgets.testing.timoni.sh"))
		g.Expect(embedded).To(ContainSubstring("status: {"))
		g.Expect(embedded).To(ContainSubstring("subresources: status: {}"))
		g.Expect(embedded).ToNot(ContainSubstring("A widget."))
		g.Expect(embedded).ToNot(ContainSubstring("The widget spec."))
		g.Expect(embedded).ToNot(ContainSubstring("Number of replicas."))
		g.Expect(embedded).To(ContainSubstring("description: type: \"string\""))
		g.Expect(embedded).To(MatchRegexp(`description:\s+"widget"`))

		g.Expect(string(generated["testing.timoni.sh/widget/v2"])).To(ContainSubstring("\n_crd: {"))
	})

	t.Run("drops the preserve-unknown-fields false markers", func(t *testing.T) {
		crd := strings.ReplaceAll(testCRDWithoutRules,
			"type: object\n          properties:", "type: object\n          x-kubernetes-preserve-unknown-fields: false\n          properties:")
		generated, err := imp.Generate([]byte(crd))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(string(generated["testing.timoni.sh/widget/v1"])).ToNot(ContainSubstring("x-kubernetes-preserve-unknown-fields"))
	})

	t.Run("fails for schemas the API server rejects", func(t *testing.T) {
		crd := strings.ReplaceAll(testCRDWithoutRules,
			"            spec:\n              type: object\n              properties:\n",
			"            spec:\n              type: object\n              patternProperties:\n                \"^x-\":\n                  type: string\n              properties:\n")
		_, err := imp.Generate([]byte(crd))
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("widgets.testing.timoni.sh"))
	})
}

func TestModuleBuilder_GetVendoredCRDs(t *testing.T) {
	g := NewWithT(t)
	widgetV1 := schema.GroupVersionKind{Group: "testing.timoni.sh", Version: "v1", Kind: "Widget"}
	widgetV2 := schema.GroupVersionKind{Group: "testing.timoni.sh", Version: "v2", Kind: "Widget"}

	imp := NewImporter(cuecontext.New(), "// Code generated by timoni. DO NOT EDIT.")
	generated, err := imp.Generate([]byte(testCRDWithRules))
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("loads schemas from packages under cue.mod/gen and cue.mod/pkg", func(t *testing.T) {
		moduleRoot := writeTestModule(t, map[string]string{
			"gen/testing.timoni.sh/widget/v1": string(generated["testing.timoni.sh/widget/v1"]),
			"pkg/testing.timoni.sh/widget/v2": string(generated["testing.timoni.sh/widget/v2"]),
		})

		builder := NewModuleBuilder(cuecontext.New(), "test", "default", moduleRoot, defaultPackage)
		_, err := builder.Build()
		g.Expect(err).ToNot(HaveOccurred())

		vendoredCRDs, err := builder.GetVendoredCRDs()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(vendoredCRDs).To(HaveLen(2))

		v := NewCRDValidator()
		g.Expect(v.AddVendoredCRDs(vendoredCRDs)).To(Succeed())
		g.Expect(v.HasSchema(widgetV1)).To(BeTrue())
		g.Expect(v.HasSchema(widgetV2)).To(BeTrue())

		errs := v.Validate(context.Background(), newTestWidget("v1", map[string]any{
			"replicas":    int64(1),
			"minReplicas": int64(5),
		}))
		g.Expect(errs).To(HaveLen(1))
		g.Expect(errs[0]).To(MatchError("spec: minReplicas must not exceed replicas"))

		object := newTestWidget("v1", map[string]any{"replicas": int64(0)})
		object.Object["status"] = map[string]any{"ready": false}
		g.Expect(v.Validate(context.Background(), object)).To(BeEmpty())
	})

	t.Run("hides the embedded schema from importers", func(t *testing.T) {
		moduleRoot := writeTestModule(t, map[string]string{
			"gen/testing.timoni.sh/widget/v1": string(generated["testing.timoni.sh/widget/v1"]),
		})
		leak := "package main\n\nimport p \"testing.timoni.sh/widget/v1\"\n\ntimoni: instance: objects: leak: p._crd\n"
		g.Expect(os.WriteFile(filepath.Join(moduleRoot, "leak.cue"), []byte(leak), 0644)).To(Succeed())

		builder := NewModuleBuilder(cuecontext.New(), "test", "default", moduleRoot, defaultPackage)
		_, err := builder.Build()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("_crd"))
	})

	t.Run("ignores hidden fields that are not CRDs", func(t *testing.T) {
		moduleRoot := writeTestModule(t, map[string]string{
			"gen/testing.timoni.sh/widget/v1": "package v1\n\n#Widget: {}\n_crd: {enabled: true}\n",
		})

		builder := NewModuleBuilder(cuecontext.New(), "test", "default", moduleRoot, defaultPackage)
		_, err := builder.Build()
		g.Expect(err).ToNot(HaveOccurred())

		vendoredCRDs, err := builder.GetVendoredCRDs()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(vendoredCRDs).To(BeEmpty())

		v := NewCRDValidator()
		g.Expect(v.AddVendoredCRDs(vendoredCRDs)).To(Succeed())
		g.Expect(v.HasSchema(widgetV1)).To(BeFalse())
	})

	t.Run("fails for malformed embedded schemas", func(t *testing.T) {
		moduleRoot := writeTestModule(t, map[string]string{
			"gen/testing.timoni.sh/widget/v1": "package v1\n\n#Widget: {}\n_crd: {apiVersion: \"apiextensions.k8s.io/v1\", kind: \"CustomResourceDefinition\", spec: versions: \"v1\"}\n",
		})

		builder := NewModuleBuilder(cuecontext.New(), "test", "default", moduleRoot, defaultPackage)
		_, err := builder.Build()
		g.Expect(err).ToNot(HaveOccurred())

		_, err = builder.GetVendoredCRDs()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("invalid _crd field in package testing.timoni.sh/widget/v1"))
	})

	t.Run("ignores packages that fail to evaluate on their own", func(t *testing.T) {
		// The generated package is imported only by the templates package,
		// whose #Route does not evaluate at the #Config defaults.
		moduleRoot := writeTestModule(t, map[string]string{})
		genDir := filepath.Join(moduleRoot, "cue.mod", "gen", "testing.timoni.sh", "widget", "v1")
		g.Expect(os.MkdirAll(genDir, os.ModePerm)).To(Succeed())
		g.Expect(os.WriteFile(filepath.Join(genDir, "types_gen.cue"), generated["testing.timoni.sh/widget/v1"], 0644)).To(Succeed())

		templates := `package templates

import p "testing.timoni.sh/widget/v1"

#Config: route: {
	enabled: *false | bool
	if enabled {
		parentRefs: [...string]
	}
}

#Route: p.#Widget & {
	_config: #Config
	spec: parentRefs: _config.route.parentRefs
}
`
		g.Expect(os.MkdirAll(filepath.Join(moduleRoot, "templates"), os.ModePerm)).To(Succeed())
		g.Expect(os.WriteFile(filepath.Join(moduleRoot, "templates", "templates.cue"), []byte(templates), 0644)).To(Succeed())
		main := "package main\n\nimport t \"timoni.sh/test/templates\"\n\n_config: t.#Config\n"
		g.Expect(os.WriteFile(filepath.Join(moduleRoot, "templates.cue"), []byte(main), 0644)).To(Succeed())

		builder := NewModuleBuilder(cuecontext.New(), "test", "default", moduleRoot, defaultPackage)
		_, err := builder.Build()
		g.Expect(err).ToNot(HaveOccurred())

		vendoredCRDs, err := builder.GetVendoredCRDs()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(vendoredCRDs).To(HaveLen(1))

		v := NewCRDValidator()
		g.Expect(v.AddVendoredCRDs(vendoredCRDs)).To(Succeed())
		g.Expect(v.HasSchema(widgetV1)).To(BeTrue())
	})

	t.Run("requires a built module", func(t *testing.T) {
		builder := NewModuleBuilder(cuecontext.New(), "test", "default", t.TempDir(), defaultPackage)
		_, err := builder.GetVendoredCRDs()
		g.Expect(err).To(MatchError("module not built"))
	})
}
