package templates

#ClientConfig: {
	_config:    #Config
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: {
		name:      "\(_config.metadata.name)-client"
		namespace: _config.metadata.namespace
		labels:    _config.metadata.labels
		if _config.metadata.annotations != _|_ {
			annotations: _config.metadata.annotations
		}
	}
	data: {
		server: "tcp://\(_config.domain):9090"
	}
}
