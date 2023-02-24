objects: [{
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: {
		name:      "test-name"
		namespace: "test-namespace"
	}
	data: {
		url: "https://test.internal"
	}
}]
