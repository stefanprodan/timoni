instance: app: {
	service: {
		apiVersion: "v1"
		kind:       "Service"
		metadata: {
			name:      "core"
			namespace: "default"
		}
		spec: {
			type: "ClusterIP"
			ports: [{
				port:       80
				protocol:   "TCP"
				targetPort: "http"
			}]
			selector: app: "core"
		}
	}
	deployment: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: {
			name:      "core"
			namespace: "default"
		}
		spec: {
			selector: matchLabels: app: "core"
			template: {
				metadata: labels: app: "core"
				spec: containers: [{
					image: "ghcr.io/stefanprodan/podinfo:6.3.0"
					name:  "podinfo"
					ports: [{
						name:          "http"
						containerPort: 9898
					}]
				}]
			}
		}
	}
}

instance: addons: {
	service: {
		apiVersion: "v1"
		kind:       "Service"
		metadata: {
			name:      "addon"
			namespace: "default"
		}
		spec: {
			type: "ClusterIP"
			ports: [{
				port:       80
				protocol:   "TCP"
				targetPort: "http"
			}]
			selector: app: "addon"
		}
	}
	deployment: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: {
			name:      "addon"
			namespace: "default"
		}
		spec: {
			selector: matchLabels: app: "addon"
			template: {
				metadata: labels: app: "addon"
				spec: containers: [{
					image: "ghcr.io/stefanprodan/podinfo:6.3.0"
					name:  "podinfo"
					ports: [{
						name:          "http"
						containerPort: 9898
					}]
				}]
			}
		}
	}
}

instance: tests: {
	service: {
		apiVersion: "v1"
		kind:       "Service"
		metadata: {
			name:      "test"
			namespace: "default"
		}
		spec: {
			type: "ClusterIP"
			ports: [{
				port:       80
				protocol:   "TCP"
				targetPort: "http"
			}]
			selector: app: "test"
		}
	}
	deployment: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: {
			name:      "test"
			namespace: "default"
		}
		spec: {
			selector: matchLabels: app: "test"
			template: {
				metadata: labels: app: "test"
				spec: containers: [{
					image: "ghcr.io/stefanprodan/podinfo:6.3.0"
					name:  "podinfo"
					ports: [{
						name:          "http"
						containerPort: 9898
					}]
				}]
			}
		}
	}
}

timoni: {
	apply: app: [ for obj in instance.app {obj}]
	apply: addons: [ for obj in instance.addons {obj}]
	apply: tests: [ for obj in instance.tests {obj}]
}
