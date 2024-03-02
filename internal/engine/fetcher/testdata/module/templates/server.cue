package templates

#ServerConfig: {
	_config:    #Config
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: {
		name:      "\(_config.metadata.name)-server"
		namespace: _config.metadata.namespace
		labels:    _config.metadata.labels
		if _config.metadata.annotations != _|_ {
			annotations: _config.metadata.annotations
		}
	}
	data: {
		hostname: "\(_config.domain)"
		port:     "9090"
	}
}
