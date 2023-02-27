values: {
	podSecurityContext: {
		runAsUser:  100
		runAsGroup: 101
		fsGroup:    101
	}
	securityContext: {
		allowPrivilegeEscalation: false
		readOnlyRootFilesystem:   true
		runAsNonRoot:             true
		capabilities: drop: ["ALL"]
		seccompProfile: type: "RuntimeDefault"
	}
}
