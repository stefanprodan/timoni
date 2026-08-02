package main

import timoniv1 "timoni.sh/core/v1alpha1"

// Define the schema for the user-supplied values.
values: {
	message: *"hello" | string
}

// Define how Timoni should build, validate and
// apply the Kubernetes resources.
timoni: {
	apiVersion: "v1alpha1"

	instance: {
		config: {
			metadata: {
				name:      string @tag(name)
				namespace: string @tag(namespace)
			}
			message: values.message
		}

		objects: {
			ns: {
				apiVersion: "v1"
				kind:       "Namespace"
				metadata: name: config.metadata.namespace
			}

			demo: {
				apiVersion: "testing.timoni.sh/v1alpha1"
				kind:       "Demo"
				metadata: {
					name:      config.metadata.name
					namespace: config.metadata.namespace
				}
				spec: message: config.message
			}
		}
	}

	apply: app: [for obj in instance.objects {obj}]

	// Unify the library checks from the vendored schemas to cover the
	// import path: the definitions must compose with the ones injected
	// by the binary, and the library maps must accept the module's own
	// check declared alongside.
	healthChecks: timoniv1.#HealthCheckLibrary.all

	// Wait for the Demo custom resource to become ready based on
	// its Ready status condition.
	healthChecks: demo: timoniv1.#HealthCheckForCondition & {
		group: "testing.timoni.sh"
		kind:  "Demo"
	}
}
