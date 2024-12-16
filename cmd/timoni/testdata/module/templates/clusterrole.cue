package templates

#ClusterRole: {
	#config:    #Config
	apiVersion: "rbac.authorization.k8s.io/v1"
	kind:       "ClusterRole"
	metadata: {
		name: "\(#config.metadata.name)-readonly"
		// This is for testing invalid namspace reference
		namespace: "default"
	}
	rules: [{
		apiGroups: [""]
		resources: ["*"]
		verbs: [
			"get",
			"list",
			"watch",
		]
	}]
}
