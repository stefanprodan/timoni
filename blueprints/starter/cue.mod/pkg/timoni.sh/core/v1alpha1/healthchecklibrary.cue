// Copyright 2026 Stefan Prodan
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// #HealthCheckLibrary provides ready-made health checks for popular
// custom resources, grouped by API family. A single family or the
// union of all families is meant to be unified into
// 'timoni: healthChecks:', where module-specific checks can be added
// alongside.
#HealthCheckLibrary: {
	// families holds the ready-made health checks grouped by API family.
	families: {
		// gatewayAPI covers the Kubernetes Gateway API kinds from both
		// the standard and the experimental channels: GatewayClass,
		// Gateway, ListenerSet and XMesh gate on their Accepted and
		// Programmed status conditions, the *Route kinds are ready when
		// every parent referenced in their spec has accepted them and
		// resolved their backend references, and the policy kinds when
		// accepted by all their ancestors. ReferenceGrant reports no
		// status and needs no check.
		gatewayAPI: {
			"gateway.networking.k8s.io/GatewayClass": #HealthCheckForCondition & {
				group:         "gateway.networking.k8s.io"
				kind:          "GatewayClass"
				conditionType: "Accepted"
			}
			"gateway.networking.k8s.io/Gateway": #HealthCheckForCondition & {
				group:         "gateway.networking.k8s.io"
				kind:          "Gateway"
				conditionType: "Programmed"
			}
			"gateway.networking.k8s.io/ListenerSet": #HealthCheckForCondition & {
				group:         "gateway.networking.k8s.io"
				kind:          "ListenerSet"
				conditionType: "Programmed"
			}
			"gateway.networking.x-k8s.io/XListenerSet": #HealthCheckForCondition & {
				group:         "gateway.networking.x-k8s.io"
				kind:          "XListenerSet"
				conditionType: "Programmed"
			}
			"gateway.networking.x-k8s.io/XMesh": #HealthCheckForCondition & {
				group:         "gateway.networking.x-k8s.io"
				kind:          "XMesh"
				conditionType: "Accepted"
			}
			for k in ["HTTPRoute", "GRPCRoute", "TCPRoute", "TLSRoute", "UDPRoute"] {
				"gateway.networking.k8s.io/\(k)": _#RouteParentsAccepted & {kind: k}
			}
			for k in ["BackendTLSPolicy", "BackendLBPolicy"] {
				"gateway.networking.k8s.io/\(k)": _#PolicyAncestorsAccepted & {
					group: "gateway.networking.k8s.io"
					kind:  k
				}
			}
			"gateway.networking.x-k8s.io/XBackendTrafficPolicy": _#PolicyAncestorsAccepted & {
				group: "gateway.networking.x-k8s.io"
				kind:  "XBackendTrafficPolicy"
			}
			...
		}

		// clusterAPI covers the Cluster API core kinds and the kubeadm
		// control plane. The v1beta1 versions report readiness with the
		// Ready status condition and v1beta2 with Available, so the
		// checks accept either. MachineSet and the provider-specific
		// bootstrap and infrastructure kinds report no common aggregate
		// readiness condition and are not covered.
		clusterAPI: {
			for k in ["Cluster", "Machine", "MachineDeployment"] {
				"cluster.x-k8s.io/\(k)": _#ReadyOrAvailableCondition & {
					group: "cluster.x-k8s.io"
					kind:  k
				}
			}
			"controlplane.cluster.x-k8s.io/KubeadmControlPlane": _#ReadyOrAvailableCondition & {
				group: "controlplane.cluster.x-k8s.io"
				kind:  "KubeadmControlPlane"
			}
			...
		}
	}

	// all unifies the health checks of every family in the library.
	all: {
		for _, family in families {family}
		...
	}
}

// _#RouteParentsAccepted is the shared health check for the Gateway API
// route kinds, which report their status per parent under
// 'status.parents' with one entry per parentRef and controller, instead
// of a top-level condition list. A route is ready when every parent
// referenced in 'spec.parentRefs' reports an 'Accepted: True' condition
// and no 'ResolvedRefs: False' one, which signals invalid backend
// references. Parents that no controller claims are never added to the
// status, so an unclaimed parent ref keeps the route in progress. As
// the status echoes the spec refs, an entry matches a parent ref when
// the fields declared in the ref are reported with equal values. A
// route with no parent refs is not attached to anything and counts as
// ready. A condition carrying its own observedGeneration only counts
// when it matches metadata.generation.
_#RouteParentsAccepted: #HealthCheck & {
	group: "gateway.networking.k8s.io"
	#object: {
		metadata: {generation?: int, ...}
		spec?: parentRefs?: [...{
			name?:        string
			namespace?:   string
			group?:       string
			kind?:        string
			sectionName?: string
			port?:        int
			...
		}]
		status?: parents?: [...{
			parentRef?: {
				name?:        string
				namespace?:   string
				group?:       string
				kind?:        string
				sectionName?: string
				port?:        int
				...
			}
			conditions?: [...{type?: string, status?: string, observedGeneration?: int, ...}]
			...
		}]
		...
	}

	_parentRefs: [
		if #object.spec.parentRefs != _|_ {#object.spec.parentRefs},
		[],
	][0]

	current: len([
		for r in _parentRefs
		if len([
			for p in #object.status.parents
			if p.parentRef.name == r.name
			if [if r.namespace != _|_ {[if p.parentRef.namespace != _|_ {p.parentRef.namespace == r.namespace}, false][0]}, true][0]
			if [if r.group != _|_ {[if p.parentRef.group != _|_ {p.parentRef.group == r.group}, false][0]}, true][0]
			if [if r.kind != _|_ {[if p.parentRef.kind != _|_ {p.parentRef.kind == r.kind}, false][0]}, true][0]
			if [if r.sectionName != _|_ {[if p.parentRef.sectionName != _|_ {p.parentRef.sectionName == r.sectionName}, false][0]}, true][0]
			if [if r.port != _|_ {[if p.parentRef.port != _|_ {p.parentRef.port == r.port}, false][0]}, true][0]
			if len([
				for c in p.conditions
				if c.type == "Accepted" && c.status == "True"
				if [
					if c.observedGeneration != _|_ {c.observedGeneration == #object.metadata.generation},
					true,
				][0] {c},
			]) > 0
			if len([
				for c in p.conditions
				if c.type == "ResolvedRefs" && c.status == "False"
				if [
					if c.observedGeneration != _|_ {c.observedGeneration == #object.metadata.generation},
					true,
				][0] {c},
			]) == 0 {p},
		]) > 0 {r},
	]) == len(_parentRefs)
}

// _#PolicyAncestorsAccepted is the shared health check for the Gateway
// API policy kinds, which report their conditions per ancestor under
// 'status.ancestors'. The set of ancestors is discovered by the
// controllers and cannot be derived from the policy spec, so a policy
// is ready when at least one ancestor is reported and every ancestor
// reports an 'Accepted: True' condition and no 'ResolvedRefs: False'
// one. A condition carrying its own observedGeneration only counts
// when it matches metadata.generation.
_#PolicyAncestorsAccepted: #HealthCheck & {
	#object: {
		metadata: {generation?: int, ...}
		status?: ancestors?: [...{
			conditions?: [...{type?: string, status?: string, observedGeneration?: int, ...}]
			...
		}]
		...
	}
	current: len(#object.status.ancestors) > 0 && len([
		for a in #object.status.ancestors
		if len([
			for c in a.conditions
			if c.type == "Accepted" && c.status == "True"
			if [
				if c.observedGeneration != _|_ {c.observedGeneration == #object.metadata.generation},
				true,
			][0] {c},
		]) > 0
		if len([
			for c in a.conditions
			if c.type == "ResolvedRefs" && c.status == "False"
			if [
				if c.observedGeneration != _|_ {c.observedGeneration == #object.metadata.generation},
				true,
			][0] {c},
		]) == 0 {a},
	]) == len(#object.status.ancestors)
}

// _#ReadyOrAvailableCondition is the shared health check for custom
// resources whose readiness condition type differs between their API
// versions, e.g. the Cluster API kinds gate on Ready in v1beta1 and on
// Available in v1beta2. As health checks match resources regardless of
// their version, the check is ready when either condition is 'True'.
// A stale status is guarded like in #HealthCheckForCondition, via the
// top-level and the per-condition observedGeneration where reported.
_#ReadyOrAvailableCondition: #HealthCheck & {
	#object: {
		metadata: {generation?: int, ...}
		status?: {
			observedGeneration?: int
			conditions?: [...{type?: string, status?: string, observedGeneration?: int, ...}]
			...
		}
		...
	}
	inProgress: [
		if #object.status.observedGeneration != _|_ {
			#object.status.observedGeneration != #object.metadata.generation
		},
		false,
	][0]
	current: len([
		for c in #object.status.conditions
		if (c.type == "Ready" || c.type == "Available") && c.status == "True"
		if [
			if c.observedGeneration != _|_ {c.observedGeneration == #object.metadata.generation},
			true,
		][0] {c},
	]) > 0
}
