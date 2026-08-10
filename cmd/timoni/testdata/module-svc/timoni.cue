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
			// The Role is applied in a separate stage before the Service.
			role: {
				apiVersion: "rbac.authorization.k8s.io/v1"
				kind:       "Role"
				metadata: {
					name:      config.metadata.name
					namespace: config.metadata.namespace
				}
				rules: [{
					apiGroups: [""]
					resources: ["configmaps"]
					verbs: ["get", "list"]
				}]
			}

			// The RoleBinding is applied in the main stage, after the
			// Role it references has registered.
			binding: {
				apiVersion: "rbac.authorization.k8s.io/v1"
				kind:       "RoleBinding"
				metadata: {
					name:      config.metadata.name
					namespace: config.metadata.namespace
				}
				roleRef: {
					apiGroup: "rbac.authorization.k8s.io"
					kind:     "Role"
					name:     config.metadata.name
				}
				subjects: [{
					kind:      "ServiceAccount"
					name:      "default"
					namespace: config.metadata.namespace
				}]
			}

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
