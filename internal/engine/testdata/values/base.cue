values: {
	resources: {
		requests: {
			cpu:    "100m"
			memory: "128Mi"
		}
		limits: memory: requests.memory
	}

	// Restricted container security context
	securityContext: {
		allowPrivilegeEscalation: false
		readOnlyRootFilesystem:   true
		runAsNonRoot:             true
		capabilities: {
			drop: ["ALL"]
			add: ["NET_BIND_SERVICE", "SYS_TIME"]
		}
	}
}
