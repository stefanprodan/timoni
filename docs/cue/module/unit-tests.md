# Unit testing

Module authors can write unit tests in CUE that check what a module renders for a
given set of values. The tests run offline with `timoni mod test`, without a
Kubernetes cluster.

## Test files

Test cases live in files ending in `_test.cue`, placed next to the templates they
cover, in the same CUE package:

```text
my-module/
├── templates/
│   ├── config.cue
│   ├── service.cue
│   └── service_test.cue
├── timoni.cue
└── values.cue
```

Like Go, CUE excludes `*_test.cue` files from regular package loads. They are
invisible to `timoni build`, `timoni apply` and `timoni mod vet`, they are read
only by `timoni mod test`, and they are never included in the published module
artifact.

## Test cases

Each case is a field under `cases`, keyed by the test name:

```cue
package templates

cases: "service defaults to ClusterIP on port 80": {
	objects: "Service/test/test": {
		apiVersion: "v1"
		spec: {
			type: "ClusterIP"
			ports: [{name: "http", port: 80, protocol: "TCP", targetPort: "http"}]
		}
	}
}
```

Timoni builds the module with the case's inputs, then unifies the case's
`objects` with the Kubernetes objects the module rendered. A case passes when
every expectation holds. Cases are independent: one case cannot influence the
outcome of another.

Only the fields named in a case are checked, so an expectation is a partial
object rather than a full manifest. Lists are an exception: their lengths must
match, so `ports: [{port: 80}]` asserts both that there is exactly one port and
that its number is 80.

Every field a case names must be one the module actually renders. An expectation
for an object or a field that does not exist fails the case:

```text
FAIL service defaults to ClusterIP on port 80
     expected field(s) "Service/test/test".spec.typ not rendered by the module
```

A case that names no expectations at all is still built, and checks that a given
set of values renders successfully.

### Addressing objects

Objects are keyed by `<kind>/<namespace>/<name>`, the same identifier Timoni
prints for a resource in the output of `timoni build`, `timoni apply` and
`timoni mod vet`. Cluster-scoped objects have no namespace segment:

```cue
objects: "Deployment/test/test": spec: replicas: 2
objects: "ClusterRole/test":     rules: [{verbs: ["get"], resources: ["pods"], apiGroups: [""]}]
```

When a module renders two objects of the same kind from different API groups,
such as a core `Service` and a Knative `Service`, the kind is qualified with the
API group. Objects in the core group have no group to add:

```cue
objects: "Service/test/test": spec: type: "ClusterIP"
objects: "Service.serving.knative.dev/test/test": spec: template: spec: containers: [{image: "app:1.0.0"}]
```

An expectation for an object the module does not render fails the case, and the
error lists the keys the module does render.

### Case fields

| Field           | Description                                                             |
|-----------------|-------------------------------------------------------------------------|
| `values`        | Values to build the module with, merged over the module's `values.cue`. |
| `objects`       | Expected Kubernetes objects, keyed by `<kind>/<namespace>/<name>`.      |
| `assert`        | Named predicates that must evaluate to `true`.                          |
| `name`          | Instance name to build with, defaults to `test`.                        |
| `namespace`     | Instance namespace to build with, defaults to `test`.                   |
| `moduleVersion` | Module version to build with.                                           |
| `kubeVersion`   | Kubernetes version to build with.                                       |

A case may declare no other field. An unknown field fails the case. Helpers and
the values under test belong in hidden fields, which are not part of a case's
field set.

### Testing with values

Set `values` to build the case with a configuration other than the module defaults:

```cue
cases: "service port is configurable": {
	values: service: port: 9898
	objects: "Service/test/test": spec: ports: [{port: 9898}]
}
```

### Testing across Kubernetes versions

Modules that select an API version based on the cluster version can pin
`kubeVersion` per case:

```cue
cases: "uses autoscaling/v2 on Kubernetes 1.23 and later": {
	values: autoscaling: enabled: true
	kubeVersion: "1.23.0"
	objects: "HorizontalPodAutoscaler/test/test": apiVersion: "autoscaling/v2"
}
```

## Assertions

Expectations that cannot be expressed as a partial object go under `assert`, where
each field is a description and its value must evaluate to `true`. A case using
`assert` must declare the objects it reads, both so that the reference resolves
and so that the keys are checked against what the module renders:

```cue
cases: "service selector targets the deployment pods": {
	objects: {
		"Service/test/test": kind:    "Service"
		"Deployment/test/test": kind: "Deployment"
	}
	assert: "selector matches the pod template labels":
		objects["Service/test/test"].spec.selector ==
		objects["Deployment/test/test"].spec.template.metadata.labels
}
```

Use assertions for cross-object invariants, for quantified checks over a list of
resources, and for the absence of a field:

```cue
cases: "service has no annotations by default": {
	objects: "Service/test/test": kind: "Service"
	assert: "annotations are absent": objects["Service/test/test"].metadata.annotations == _|_
}
```

A path that does not resolve is bottom, and bottom satisfies `== _|_`, so an
absence assertion holds whether the field is absent or its name is misspelled.
Pair it with a positive assertion on the same object when the distinction
matters.

## Testing the config schema

A module's `#Config` is its public contract, and the values it rejects are worth
testing. Because the config is reachable from the test file's package, these cases
need no rendering at all:

```cue
cases: "service port outside the valid range is rejected": {
	_config: #Config & {service: port: 70000}
	assert: "port 70000 is rejected": _config == _|_
}
```

The value under test must be assigned to a hidden field, so that the intentional
error does not affect the rest of the evaluation.

Note that a missing required field is incomplete rather than invalid, so it does
not satisfy `== _|_`. To check that a field is required, assert on the field the
schema leaves unresolved instead.

## Running tests

```shell
timoni mod test ./path/to/module
```

```text
PASS service annotations are propagated
PASS service defaults to ClusterIP on port 80
PASS service has no annotations by default
PASS service port is configurable
PASS service port outside the valid range is rejected
PASS service selector targets the deployment pods
OK 6 test cases passed
```

The command exits non-zero when a case fails. Every expectation that did not
hold is reported, with the path to the field and its position in the test file:

```text
FAIL service defaults to ClusterIP on port 80
     cases."service defaults to ClusterIP on port 80".objects."Service/test/test".spec.ports.0.port: conflicting values 80 and 443:
         ./templates/service_test.cue:6:33
     cases."service defaults to ClusterIP on port 80".objects."Service/test/test".spec.type: conflicting values "ClusterIP" and "LoadBalancer":
         ./templates/service_test.cue:5:9
```

To run a subset of the cases, filter them by name with a regular expression:

```shell
timoni mod test ./path/to/module --run '^service'
```

A module with no test cases at all passes. An expression that matches none of
the cases a module does have is an error.
