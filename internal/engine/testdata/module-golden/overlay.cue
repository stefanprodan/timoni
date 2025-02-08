objects: [{
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: {
		name:      "test-name"
		namespace: "test-namespace"
	}
	data: {
		url:           "https://test.internal"
		moduleVersion: "0.0.0-devel"
		kubeVersion:   "1.25.3"
	}
}]
