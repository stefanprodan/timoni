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
	"time"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const demoCRD = `
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: demos.testing.timoni.sh
spec:
  group: testing.timoni.sh
  names:
    kind: Demo
    listKind: DemoList
    plural: demos
    singular: demo
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      subresources:
        status: {}
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                message:
                  type: string
            status:
              type: object
              properties:
                observedGeneration:
                  type: integer
                conditions:
                  type: array
                  items:
                    type: object
                    properties:
                      type:
                        type: string
                      status:
                        type: string
                      observedGeneration:
                        type: integer
                      reason:
                        type: string
                      message:
                        type: string
`

func TestApplyWithHealthChecks(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()
	modPath := "testdata/module-hc"
	namespace := rnd("my-namespace")

	// Install the Demo CRD used by the module's health check.
	crd := &unstructured.Unstructured{}
	g.Expect(yaml.Unmarshal([]byte(demoCRD), &crd.Object)).To(Succeed())
	g.Expect(envTestClient.Create(ctx, crd)).To(Succeed())

	// Wait for the CRD to be served.
	g.Eventually(func() error {
		list := &unstructured.UnstructuredList{}
		list.SetAPIVersion("testing.timoni.sh/v1alpha1")
		list.SetKind("DemoList")
		return envTestClient.List(ctx, list)
	}, 10*time.Second).Should(Succeed())

	getDemo := func(name string) (*unstructured.Unstructured, error) {
		demo := &unstructured.Unstructured{}
		demo.SetAPIVersion("testing.timoni.sh/v1alpha1")
		demo.SetKind("Demo")
		demo.SetName(name)
		demo.SetNamespace(namespace)
		err := envTestClient.Get(ctx, client.ObjectKeyFromObject(demo), demo)
		return demo, err
	}

	// setDemoStatus reports the given condition type as True
	// for the current generation of the Demo custom resource.
	setDemoStatus := func(name, conditionType string) error {
		demo, err := getDemo(name)
		if err != nil {
			return err
		}
		gen := demo.GetGeneration()
		status := map[string]any{
			"observedGeneration": gen,
			"conditions": []any{
				map[string]any{
					"type":               conditionType,
					"status":             "True",
					"observedGeneration": gen,
					"reason":             "Testing",
					"message":            "set by the test",
				},
			},
		}
		if err := unstructured.SetNestedMap(demo.Object, status, "status"); err != nil {
			return err
		}
		return envTestClient.Status().Update(ctx, demo)
	}

	applyInstance := func(name string) chan error {
		result := make(chan error, 1)
		go func() {
			_, err := executeCommand(fmt.Sprintf(
				"apply -n %s %s %s -p main --wait --timeout=30s",
				namespace,
				name,
				modPath,
			))
			result <- err
		}()
		return result
	}

	t.Run("waits for custom resource readiness", func(t *testing.T) {
		g := NewWithT(t)
		name := rnd("my-demo")

		applyResult := applyInstance(name)

		// The Demo object gets applied but the apply command must
		// keep waiting as the health check is not passing yet.
		g.Eventually(func() error {
			_, err := getDemo(name)
			return err
		}, 10*time.Second).Should(Succeed())
		g.Consistently(applyResult, 2*time.Second).ShouldNot(Receive())

		// Report the resource as ready and expect the apply to finish.
		g.Eventually(func() error {
			return setDemoStatus(name, "Ready")
		}, 5*time.Second).Should(Succeed())

		var applyErr error
		g.Eventually(applyResult, 20*time.Second).Should(Receive(&applyErr))
		g.Expect(applyErr).ToNot(HaveOccurred())
	})

	t.Run("fails fast on stalled custom resource", func(t *testing.T) {
		g := NewWithT(t)
		name := rnd("my-demo")

		applyResult := applyInstance(name)

		g.Eventually(func() error {
			_, err := getDemo(name)
			return err
		}, 10*time.Second).Should(Succeed())

		// Report the resource as stalled and expect the apply to
		// fail before the wait timeout expires.
		g.Eventually(func() error {
			return setDemoStatus(name, "Stalled")
		}, 5*time.Second).Should(Succeed())

		var applyErr error
		g.Eventually(applyResult, 20*time.Second).Should(Receive(&applyErr))
		g.Expect(applyErr).To(HaveOccurred())
		g.Expect(applyErr.Error()).To(ContainSubstring("Demo"))
	})
}
