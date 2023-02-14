values: {
	metadata: {
		name:      "podinfo"
		namespace: "default"
	}

	resources: requests: {
		cpu:    "100m"
		memory: "128Mi"
	}
	resources: limits: resources.requests

	autoscaling: {
		enabled:     true
		minReplicas: 2
		maxReplicas: 10
		cpu:         90
		memory:      "100Mi"
	}
}
