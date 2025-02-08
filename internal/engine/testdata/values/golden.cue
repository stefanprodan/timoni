values: {
	securityContext: {
		allowPrivilegeEscalation: false
		capabilities: {
			add: ["NET_BIND_SERVICE"]
			drop: ["ALL"]
		}
		readOnlyRootFilesystem: false
		runAsNonRoot:           true
		seccompProfile: {
			type: "RuntimeDefault"
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
