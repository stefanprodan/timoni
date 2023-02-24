values: {
	resources: limits: {
		cpu:    "1000m"
		memory: "1Gi"
	}

	securityContext: {
		readOnlyRootFilesystem: false
		capabilities: add: ["NET_BIND_SERVICE"]
	}
}
