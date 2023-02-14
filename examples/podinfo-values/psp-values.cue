values: {
	podSecurityContext: {
		runAsUser:  1000
		runAsGroup: 3000
		fsGroup:    2000
	}
	securityContext: {
		allowPrivilegeEscalation: false
		readOnlyRootFilesystem:   true
		runAsNonRoot:             true
		capabilities: drop: ["ALL"]
		seccompProfile: type: "RuntimeDefault"
	}
}
