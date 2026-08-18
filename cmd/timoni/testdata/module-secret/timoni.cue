package main

// Define the schema for the user-supplied values.
values: {
	password: *"s3cr3t" | string
	token:    *"t0k3n" | string
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
			password: values.password
			token:    values.token
		}

		objects: {
			// The ConfigMap references the Secret and must never be masked.
			cm: {
				apiVersion: "v1"
				kind:       "ConfigMap"
				metadata: {
					name:      config.metadata.name
					namespace: config.metadata.namespace
				}
				data: secretName: config.metadata.name
			}

			secret: {
				apiVersion: "v1"
				kind:       "Secret"
				metadata: {
					name:      config.metadata.name
					namespace: config.metadata.namespace
				}
				type: "Opaque"
				stringData: {
					password: config.password
					token:    config.token
				}
			}
		}
	}

	apply: app: [for obj in instance.objects {obj}]
}
