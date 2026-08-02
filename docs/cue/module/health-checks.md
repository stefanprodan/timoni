# Custom Health Checks

When applying an instance with `--wait`, Timoni waits for all the applied
resources to become ready. Readiness is determined with
[Flux kstatus](https://github.com/fluxcd/cli-utils),
which understands the Kubernetes built-in kinds and custom resources that
follow the kstatus conventions.

For custom resources that are not kstatus-compliant, module authors can
define their own readiness evaluation in CUE with the
`timoni: healthChecks:` field.

The majority of custom resources signal readiness through status
conditions, and for these a health check is a one-line declaration using
the [`#HealthCheckForCondition`](#condition-based-health-checks)
shorthand; resources with bespoke status reporting use the raw
`#HealthCheck` form, which takes the readiness evaluation as CUE
expressions.

## Defining health checks

Health checks are declared in the module's root CUE package, either in
`timoni.cue` next to `timoni: apply:`, or in a dedicated sibling file
e.g. `healthchecks.cue`.

For example, waiting for a Kubernetes
[Gateway](https://gateway-api.sigs.k8s.io) to be programmed by its
controller and for an HTTPRoute to be accepted by all its parent
gateways. The Gateway readiness comes from its `Programmed` status
condition, so the shorthand fits. The HTTPRoute needs a raw check, as the
Gateway API routes report their conditions per parent gateway instead
of a top-level status condition:

```cue
package main

import timoniv1 "timoni.sh/core/v1alpha1"

timoni: healthChecks: {
	"gateway.networking.k8s.io/Gateway": timoniv1.#HealthCheckForCondition & {
		group:         "gateway.networking.k8s.io"
		kind:          "Gateway"
		conditionType: "Programmed"
	}
	"gateway.networking.k8s.io/HTTPRoute": timoniv1.#HealthCheck & {
		group: "gateway.networking.k8s.io"
		kind:  "HTTPRoute"
		#object: status?: parents?: [...{
			conditions?: [...{type?: string, status?: string, ...}]
			...
		}]
		current: len(#object.status.parents) > 0 && len([
			for p in #object.status.parents
			for c in p.conditions
			if c.type == "Accepted" && c.status == "True" {c},
		]) == len(#object.status.parents)
	}
}
```

Each health check targets the resources matching its `group` and `kind`,
regardless of their version. When `kind` is omitted, the check applies to
every kind in the group. A check with a specific `kind` takes precedence
over a group-wide one, and two checks must not target the same group and
kind. Health checks are meant for custom resources; the Kubernetes
built-in kinds are covered by kstatus.

Note that `#HealthCheck` is an open definition so that shorthands like
`#HealthCheckForCondition` can extend it with extra inputs. The `#HealthCheck`,
`#HealthCheckForCondition` are injected into the module's package at build time,
so these names are reserved and must not be redefined by the module.

While waiting, Timoni fills `#object` with the live object read from the
cluster and evaluates the boolean expressions in order, where the first
one that evaluates to `true` decides the resource status:

1. `inProgress`: the resource is reconciling, keep waiting.
2. `failed`: the resource reached a terminal failure state, abort the wait early.
3. `current`: the resource is ready.
4. When no expression evaluates to `true`, the resource counts as
   reconciling and is polled until it becomes ready or the timeout expires.

An expression that cannot be evaluated because the referenced fields are
missing from the live object counts as `false`. A freshly created resource
with no `status` is therefore simply in progress until its controller
reports one, and `current` is the only expression that has to be defined.

## Condition-based health checks

For custom resources that signal readiness through status conditions,
the `timoniv1.#HealthCheckForCondition` shorthand generates the
expressions from two inputs: `conditionType` (default `Ready`) gates
readiness and `failedConditionType` (default `Stalled`) triggers the
fail-fast behaviour.

The shorthand checks generation staleness wherever the resource tracks
it, and skips the check where it doesn't:

- A top-level `status.observedGeneration` lagging `metadata.generation`
  reports the resource as reconciling.
- A condition carrying its own `observedGeneration` only counts as ready
  or failed when it matches `metadata.generation`, so a stale condition
  left over from the previous spec never decides the outcome.
- Resources with neither simply gate on the condition status.
- Resources that never emit a `failedConditionType` condition never fail fast.

### Examples

Waiting for a [SealedSecret](https://github.com/bitnami-labs/sealed-secrets)
to be unsealed, based on its `Synced` status condition:

```cue
timoni: healthChecks: {
	"bitnami.com/SealedSecret": timoniv1.#HealthCheckForCondition & {
		group:         "bitnami.com"
		kind:          "SealedSecret"
		conditionType: "Synced"
	}
}
```

Waiting for the [Flux Operator](https://fluxcd.control-plane.io/operator/)
custom resources with a group-wide check, for which the shorthand
defaults fit exactly, as these resources report readiness with `Ready`
and terminal failures with `Stalled`:

```cue
timoni: healthChecks: {
	"fluxcd.controlplane.io": timoniv1.#HealthCheckForCondition & {
		group: "fluxcd.controlplane.io"
	}
}
```

Waiting for a [Rook Ceph](https://rook.io) cluster, whose readiness comes
from the live cluster health field instead of conditions and thus needs
the raw form. `CephCluster` reports a top-level `status.observedGeneration`,
so stale status after an upgrade is guarded via `inProgress`, which is
evaluated before `failed`, ensuring a leftover `HEALTH_ERR` can't fail
a fresh rollout:

```cue
timoni: healthChecks: {
	"ceph.rook.io/CephCluster": timoniv1.#HealthCheck & {
		group: "ceph.rook.io"
		kind:  "CephCluster"
		#object: {
			metadata: {generation?: int, ...}
			status?: {
				observedGeneration?: int
				ceph?: {health?: string, ...}
				...
			}
			...
		}
		inProgress: #object.status.observedGeneration != #object.metadata.generation
		current:    #object.status.ceph.health == "HEALTH_OK"
		failed:     #object.status.ceph.health == "HEALTH_ERR"
	}
}
```

## Typing the object

The vendored CUE schemas generated by
[`timoni mod vendor crd`](custom-resources.md) contain only the `spec` of
custom resources, so health check expressions reference the status via the
`#object` constraints declared inline, as in the examples above. Keep the
constraints open (`...`) and the status fields optional (`?`), otherwise
live objects with additional or missing fields would fail the evaluation
with a schema error.

Note that resources annotated with
[`action.timoni.sh/wait: disabled`](apply-behavior.md#disable-waiting)
are excluded from waiting and their health checks are not evaluated.
