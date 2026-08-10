package main

// Define the schema for the user-supplied values.
values: {
	port: *80 | int
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
			port: values.port
		}

		objects: {
			svc: {
				apiVersion: "v1"
				kind:       "Service"
				metadata: {
					name:      config.metadata.name
					namespace: config.metadata.namespace
				}
				spec: {
					type: "ClusterIP"
					selector: app: config.metadata.name
					ports: [{
						name:       "http"
						port:       config.port
						protocol:   "TCP"
						targetPort: config.port
					}]
				}
			}
		}
	}

	apply: app: [for obj in instance.objects {obj}]
}
