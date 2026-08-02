package main

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

	// Wait for the Demo custom resource to become ready based on
	// its Ready status condition.
	healthChecks: demo: #HealthCheckForCondition & {
		group: "testing.timoni.sh"
		kind:  "Demo"
	}
}
