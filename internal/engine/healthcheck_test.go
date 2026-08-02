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
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/schemas"
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

func TestHealthCheckLibrary(t *testing.T) {
	g := NewWithT(t)

	// The library is not injected into module packages: load its schema
	// file as if the module had imported the vendored CUE package.
	libData, err := schemas.FS.ReadFile("timoni.sh/core/v1alpha1/healthchecklibrary.cue")
	g.Expect(err).ToNot(HaveOccurred())
	lib := strings.Replace(string(libData), "package v1alpha1", "", 1)

	// A module-specific check declared alongside the library ones must
	// unify: the library check maps are explicitly open, as values
	// extracted from a definition are otherwise closed and would reject
	// any sibling field.
	checks, err := getHealthChecks(t, lib+`
timoni: {
	apiVersion: "v1alpha1"
	instance: {}
	apply: {}
	healthChecks: #HealthCheckLibrary.all
	healthChecks: "example.com/Database": #HealthCheck & {
		group: "example.com"
		kind:  "Database"
		#object: status?: {phase?: string, ...}
		current: #object.status.phase == "Ready"
	}
}
`)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checks).To(HaveLen(18))

	byKind := make(map[string]*HealthCheck, len(checks))
	for _, hc := range checks {
		byKind[hc.GroupKind.Kind] = hc
	}
	for _, kind := range []string{"GatewayClass", "Gateway", "ListenerSet", "HTTPRoute", "GRPCRoute", "TCPRoute", "TLSRoute", "UDPRoute", "BackendTLSPolicy", "BackendLBPolicy"} {
		g.Expect(byKind).To(HaveKey(kind))
		g.Expect(byKind[kind].GroupKind.Group).To(BeEquivalentTo("gateway.networking.k8s.io"))
	}
	for _, kind := range []string{"XListenerSet", "XBackendTrafficPolicy", "XMesh"} {
		g.Expect(byKind).To(HaveKey(kind))
		g.Expect(byKind[kind].GroupKind.Group).To(BeEquivalentTo("gateway.networking.x-k8s.io"))
	}
	for _, kind := range []string{"Cluster", "Machine", "MachineDeployment"} {
		g.Expect(byKind).To(HaveKey(kind))
		g.Expect(byKind[kind].GroupKind.Group).To(BeEquivalentTo("cluster.x-k8s.io"))
	}
	g.Expect(byKind).To(HaveKey("KubeadmControlPlane"))
	g.Expect(byKind["KubeadmControlPlane"].GroupKind.Group).To(BeEquivalentTo("controlplane.cluster.x-k8s.io"))

	condition := func(t, s string, gen int64) map[string]any {
		return map[string]any{"type": t, "status": s, "observedGeneration": gen}
	}
	ref := func(name string) map[string]any {
		return map[string]any{"name": name, "group": "gateway.networking.k8s.io", "kind": "Gateway"}
	}
	parent := func(parentRef map[string]any, conditions ...any) map[string]any {
		return map[string]any{"parentRef": parentRef, "conditions": conditions}
	}

	tests := []struct {
		name   string
		kind   string
		object map[string]any
		expect HealthStatus
	}{
		{
			name: "programmed gateway is current",
			kind: "Gateway",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(1)},
				"status": map[string]any{
					"conditions": []any{condition("Accepted", "True", 1), condition("Programmed", "True", 1)},
				},
			},
			expect: HealthStatusCurrent,
		},
		{
			name: "accepted gateway class is current",
			kind: "GatewayClass",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(1)},
				"status": map[string]any{
					"conditions": []any{condition("Accepted", "True", 1)},
				},
			},
			expect: HealthStatusCurrent,
		},
		{
			name: "route accepted by all spec parents is current",
			kind: "HTTPRoute",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"spec":     map[string]any{"parentRefs": []any{ref("gw-a"), ref("gw-b")}},
				"status": map[string]any{
					"parents": []any{
						parent(ref("gw-a"), condition("Accepted", "True", 2), condition("ResolvedRefs", "True", 2)),
						parent(ref("gw-b"), condition("Accepted", "True", 2)),
					},
				},
			},
			expect: HealthStatusCurrent,
		},
		{
			name: "route rejected by one parent is in progress",
			kind: "HTTPRoute",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"spec":     map[string]any{"parentRefs": []any{ref("gw-a"), ref("gw-b")}},
				"status": map[string]any{
					"parents": []any{
						parent(ref("gw-a"), condition("Accepted", "True", 2)),
						parent(ref("gw-b"), condition("Accepted", "False", 2)),
					},
				},
			},
			expect: HealthStatusInProgress,
		},
		{
			name: "route with an unclaimed parent is in progress",
			kind: "HTTPRoute",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"spec":     map[string]any{"parentRefs": []any{ref("gw-a"), ref("gw-b")}},
				"status": map[string]any{
					"parents": []any{
						parent(ref("gw-a"), condition("Accepted", "True", 2)),
					},
				},
			},
			expect: HealthStatusInProgress,
		},
		{
			name: "route with one parent claimed by two controllers and one unclaimed is in progress",
			kind: "HTTPRoute",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"spec":     map[string]any{"parentRefs": []any{ref("gw-a"), ref("gw-b")}},
				"status": map[string]any{
					"parents": []any{
						parent(ref("gw-a"), condition("Accepted", "True", 2)),
						parent(ref("gw-a"), condition("Accepted", "True", 2)),
					},
				},
			},
			expect: HealthStatusInProgress,
		},
		{
			name: "route with unresolved backend refs is in progress",
			kind: "HTTPRoute",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"spec":     map[string]any{"parentRefs": []any{ref("gw-a")}},
				"status": map[string]any{
					"parents": []any{
						parent(ref("gw-a"), condition("Accepted", "True", 2), condition("ResolvedRefs", "False", 2)),
					},
				},
			},
			expect: HealthStatusInProgress,
		},
		{
			name: "route with section-specific parents is current",
			kind: "HTTPRoute",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"spec": map[string]any{"parentRefs": []any{
					map[string]any{"name": "gw", "sectionName": "http"},
					map[string]any{"name": "gw", "sectionName": "https"},
				}},
				"status": map[string]any{
					"parents": []any{
						parent(map[string]any{"name": "gw", "sectionName": "http"}, condition("Accepted", "True", 2)),
						parent(map[string]any{"name": "gw", "sectionName": "https"}, condition("Accepted", "True", 2)),
					},
				},
			},
			expect: HealthStatusCurrent,
		},
		{
			name: "route accepted by its mesh service parent is current",
			kind: "HTTPRoute",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"spec": map[string]any{"parentRefs": []any{
					map[string]any{"name": "backend", "group": "", "kind": "Service"},
				}},
				"status": map[string]any{
					"parents": []any{
						parent(map[string]any{"name": "backend", "group": "", "kind": "Service"}, condition("Accepted", "True", 2)),
					},
				},
			},
			expect: HealthStatusCurrent,
		},
		{
			name: "route with no parent refs is current",
			kind: "HTTPRoute",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"spec":     map[string]any{},
			},
			expect: HealthStatusCurrent,
		},
		{
			name: "route with no status is in progress",
			kind: "HTTPRoute",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"spec":     map[string]any{"parentRefs": []any{ref("gw-a")}},
			},
			expect: HealthStatusInProgress,
		},
		{
			name: "route with stale accepted condition is in progress",
			kind: "HTTPRoute",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"spec":     map[string]any{"parentRefs": []any{ref("gw-a")}},
				"status": map[string]any{
					"parents": []any{
						parent(ref("gw-a"), condition("Accepted", "True", 1)),
					},
				},
			},
			expect: HealthStatusInProgress,
		},
		{
			name: "programmed listener set is current",
			kind: "ListenerSet",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(1)},
				"status": map[string]any{
					"conditions": []any{condition("Accepted", "True", 1), condition("Programmed", "True", 1)},
				},
			},
			expect: HealthStatusCurrent,
		},
		{
			name: "programmed experimental listener set is current",
			kind: "XListenerSet",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(1)},
				"status": map[string]any{
					"conditions": []any{condition("Accepted", "True", 1), condition("Programmed", "True", 1)},
				},
			},
			expect: HealthStatusCurrent,
		},
		{
			name: "pending mesh is in progress",
			kind: "XMesh",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(1)},
				"status": map[string]any{
					"conditions": []any{map[string]any{"type": "Accepted", "status": "Unknown", "reason": "Pending"}},
				},
			},
			expect: HealthStatusInProgress,
		},
		{
			name: "policy accepted by all ancestors is current",
			kind: "BackendTLSPolicy",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(1)},
				"status": map[string]any{
					"ancestors": []any{
						map[string]any{"conditions": []any{condition("Accepted", "True", 1)}},
						map[string]any{"conditions": []any{condition("Accepted", "True", 1)}},
					},
				},
			},
			expect: HealthStatusCurrent,
		},
		{
			name: "policy rejected by one ancestor is in progress",
			kind: "XBackendTrafficPolicy",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(1)},
				"status": map[string]any{
					"ancestors": []any{
						map[string]any{"conditions": []any{condition("Accepted", "True", 1)}},
						map[string]any{"conditions": []any{condition("Accepted", "False", 1)}},
					},
				},
			},
			expect: HealthStatusInProgress,
		},
		{
			name: "policy with unresolved refs is in progress",
			kind: "BackendTLSPolicy",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(1)},
				"status": map[string]any{
					"ancestors": []any{
						map[string]any{"conditions": []any{condition("Accepted", "True", 1), condition("ResolvedRefs", "False", 1)}},
					},
				},
			},
			expect: HealthStatusInProgress,
		},
		{
			name: "v1beta1 ready cluster is current",
			kind: "Cluster",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(1)},
				"status": map[string]any{
					"observedGeneration": int64(1),
					"conditions":         []any{map[string]any{"type": "Ready", "status": "True"}},
				},
			},
			expect: HealthStatusCurrent,
		},
		{
			name: "v1beta2 available cluster is current",
			kind: "Cluster",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(1)},
				"status": map[string]any{
					"observedGeneration": int64(1),
					"conditions":         []any{condition("Available", "True", 1)},
				},
			},
			expect: HealthStatusCurrent,
		},
		{
			name: "provisioning control plane is in progress",
			kind: "KubeadmControlPlane",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(1)},
				"status": map[string]any{
					"observedGeneration": int64(1),
					"conditions":         []any{condition("Available", "False", 1)},
				},
			},
			expect: HealthStatusInProgress,
		},
		{
			name: "machine deployment with stale status is in progress",
			kind: "MachineDeployment",
			object: map[string]any{
				"metadata": map[string]any{"generation": int64(2)},
				"status": map[string]any{
					"observedGeneration": int64(1),
					"conditions":         []any{condition("Available", "True", 1)},
				},
			},
			expect: HealthStatusInProgress,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			res, err := byKind[tt.kind].Evaluate(tt.object)
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
