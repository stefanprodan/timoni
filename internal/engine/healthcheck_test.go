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
	"k8s.io/apimachinery/pkg/runtime/schema"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func getHealthChecks(t *testing.T, src string) ([]*HealthCheck, error) {
	t.Helper()

	ctx := cuecontext.New()
	value := ctx.CompileString(apiv1.InstanceSchema + src)
	if err := value.Err(); err != nil {
		return nil, err
	}

	b := &ModuleBuilder{}
	return b.GetHealthChecks(value)
}

func TestGetHealthChecks(t *testing.T) {
	g := NewWithT(t)

	checks, err := getHealthChecks(t, `
timoni: {
	apiVersion: "v1alpha1"
	instance: {}
	apply: {}
	healthChecks: {
		"example.com/Database": #HealthCheck & {
			group: "example.com"
			kind:  "Database"
			#object: status?: {phase?: string, ...}
			current: #object.status.phase == "Ready"
			failed:  #object.status.phase == "Failed"
		}
		"testing.timoni.sh/Demo": #HealthCheckForCondition & {
			group: "testing.timoni.sh"
			kind:  "Demo"
		}
		"fluxcd.controlplane.io": #HealthCheckForCondition & {
			group: "fluxcd.controlplane.io"
		}
	}
}
`)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checks).To(HaveLen(3))

	g.Expect(checks[0].Name).To(BeEquivalentTo("example.com/Database"))
	g.Expect(checks[0].GroupKind).To(BeEquivalentTo(schema.GroupKind{Group: "example.com", Kind: "Database"}))

	g.Expect(checks[1].GroupKind).To(BeEquivalentTo(schema.GroupKind{Group: "testing.timoni.sh", Kind: "Demo"}))

	// A health check without a kind applies to the whole group.
	g.Expect(checks[2].GroupKind).To(BeEquivalentTo(schema.GroupKind{Group: "fluxcd.controlplane.io"}))
}

func TestGetHealthChecks_Absent(t *testing.T) {
	g := NewWithT(t)

	checks, err := getHealthChecks(t, `
timoni: {
	apiVersion: "v1alpha1"
	instance: {}
	apply: {}
}
`)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checks).To(BeNil())
}

func TestGetHealthChecks_DuplicateGroupKind(t *testing.T) {
	g := NewWithT(t)

	_, err := getHealthChecks(t, `
timoni: {
	apiVersion: "v1alpha1"
	instance: {}
	apply: {}
	healthChecks: {
		"one": #HealthCheckForCondition & {
			group: "example.com"
			kind:  "Database"
		}
		"two": #HealthCheckForCondition & {
			group: "example.com"
			kind:  "Database"
		}
	}
}
`)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("same GroupKind"))
}

func TestGetHealthChecks_MissingCurrent(t *testing.T) {
	g := NewWithT(t)

	_, err := getHealthChecks(t, `
timoni: {
	apiVersion: "v1alpha1"
	instance: {}
	apply: {}
	healthChecks: {
		"example.com/Database": #HealthCheck & {
			group: "example.com"
			kind:  "Database"
			#object: status?: {phase?: string, ...}
			failed: #object.status.phase == "Failed"
		}
	}
}
`)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("current"))
}

func TestGetHealthChecks_InvalidGroup(t *testing.T) {
	g := NewWithT(t)

	// A group written in the '<group>/<version>' apiVersion format is
	// rejected by the schema constraint.
	_, err := getHealthChecks(t, `
timoni: {
	apiVersion: "v1alpha1"
	instance: {}
	apply: {}
	healthChecks: {
		"example.com/Database": #HealthCheck & {
			group: "example.com/v1"
			kind:  "Database"
			#object: {...}
			current: true
		}
	}
}
`)
	g.Expect(err).To(HaveOccurred())
}

func TestHealthCheck_Evaluate(t *testing.T) {
	g := NewWithT(t)

	checks, err := getHealthChecks(t, `
timoni: {
	apiVersion: "v1alpha1"
	instance: {}
	apply: {}
	healthChecks: {
		"example.com/Database": #HealthCheck & {
			group: "example.com"
			kind:  "Database"
			#object: status?: {phase?: string, ...}
			current: #object.status.phase == "Ready"
			failed:  #object.status.phase == "Failed"
		}
		"testing.timoni.sh/Demo": #HealthCheckForCondition & {
			group: "testing.timoni.sh"
			kind:  "Demo"
		}
	}
}
`)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checks).To(HaveLen(2))
	database, demo := checks[0], checks[1]

	condition := func(t, s string, gen int64) map[string]any {
		return map[string]any{"type": t, "status": s, "observedGeneration": gen}
	}

	tests := []struct {
		name   string
		check  *HealthCheck
		object map[string]any
		expect HealthStatus
	}{
		{
			name:   "no status is in progress",
			check:  database,
			object: map[string]any{"metadata": map[string]any{"name": "test"}},
			expect: HealthStatusInProgress,
		},
		{
			name:   "matching current expression",
			check:  database,
			object: map[string]any{"status": map[string]any{"phase": "Ready"}},
			expect: HealthStatusCurrent,
		},
		{
			name:   "matching failed expression",
			check:  database,
			object: map[string]any{"status": map[string]any{"phase": "Failed"}},
			expect: HealthStatusFailed,
		},
		{
			name:   "unknown phase is in progress",
			check:  database,
			object: map[string]any{"status": map[string]any{"phase": "Provisioning"}},
			expect: HealthStatusInProgress,
		},
		{
			name:  "fresh ready condition is current",
			check: demo,
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"status": map[string]any{
					"conditions": []any{condition("Ready", "True", 2)},
				},
			},
			expect: HealthStatusCurrent,
		},
		{
			name:  "stale ready condition is in progress",
			check: demo,
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"status": map[string]any{
					"conditions": []any{condition("Ready", "True", 1)},
				},
			},
			expect: HealthStatusInProgress,
		},
		{
			name:  "ready condition without generation tracking is current",
			check: demo,
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"status": map[string]any{
					"conditions": []any{map[string]any{"type": "Ready", "status": "True"}},
				},
			},
			expect: HealthStatusCurrent,
		},
		{
			name:  "stale top-level observedGeneration is in progress",
			check: demo,
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"status": map[string]any{
					"observedGeneration": int64(1),
					"conditions":         []any{condition("Ready", "True", 2)},
				},
			},
			expect: HealthStatusInProgress,
		},
		{
			name:  "fresh stalled condition is failed",
			check: demo,
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"status": map[string]any{
					"conditions": []any{condition("Stalled", "True", 2), condition("Ready", "False", 2)},
				},
			},
			expect: HealthStatusFailed,
		},
		{
			name:  "stale stalled condition is in progress",
			check: demo,
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"status": map[string]any{
					"conditions": []any{condition("Stalled", "True", 1)},
				},
			},
			expect: HealthStatusInProgress,
		},
		{
			name:  "false ready condition is in progress",
			check: demo,
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"status": map[string]any{
					"conditions": []any{condition("Ready", "False", 2)},
				},
			},
			expect: HealthStatusInProgress,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			res, err := tt.check.Evaluate(tt.object)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(res).To(BeEquivalentTo(tt.expect))
		})
	}
}

func TestHealthCheck_EvaluateError(t *testing.T) {
	g := NewWithT(t)

	checks, err := getHealthChecks(t, `
timoni: {
	apiVersion: "v1alpha1"
	instance: {}
	apply: {}
	healthChecks: {
		"example.com/Database": #HealthCheck & {
			group: "example.com"
			kind:  "Database"
			#object: status?: {phase?: string, ...}
			current: #object.status.phase == "Ready"
		}
	}
}
`)
	g.Expect(err).ToNot(HaveOccurred())

	// The live object conflicts with the #object schema.
	_, err = checks[0].Evaluate(map[string]any{
		"status": map[string]any{"phase": int64(5)},
	})
	g.Expect(err).To(HaveOccurred())
}
