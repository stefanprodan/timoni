values: {
	securityContext: {
		seccompProfile: {
			type: "RuntimeDefault"
		}
		readOnlyRootFilesystem:   false
		allowPrivilegeEscalation: false
		runAsNonRoot:             true
		capabilities: {
			add: ["NET_BIND_SERVICE"]
			drop: ["ALL"]
		}
	}
	resources: {
		limits: {
			cpu:    "1000m"
			memory: "1Gi"
		}
		requests: {
			cpu:    "100m"
			memory: "128Mi"
		}
	}
}
