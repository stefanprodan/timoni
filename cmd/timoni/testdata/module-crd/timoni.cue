package main

// Define the schema for the user-supplied values.
values: {
	group: *"testing.timoni.sh" | string
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
			group: values.group
		}

		objects: {
			// The CRD and a custom resource of its kind belong to the
			// same apply set: the staged apply must register the CRD
			// before resolving the Widget kind.
			crd: {
				apiVersion: "apiextensions.k8s.io/v1"
				kind:       "CustomResourceDefinition"
				metadata: name: "widgets.\(config.group)"
				spec: {
					group: config.group
					names: {
						kind:     "Widget"
						listKind: "WidgetList"
						plural:   "widgets"
						singular: "widget"
					}
					scope: "Namespaced"
					versions: [{
						name:    "v1"
						served:  true
						storage: true
						schema: openAPIV3Schema: {
							type: "object"
							properties: spec: {
								type: "object"
								properties: message: type: "string"
							}
						}
					}]
				}
			}

			widget: {
				apiVersion: "\(config.group)/v1"
				kind:       "Widget"
				metadata: {
					name:      config.metadata.name
					namespace: config.metadata.namespace
				}
				spec: message: "hello"
			}
		}
	}

	apply: app: [for obj in instance.objects {obj}]
}
