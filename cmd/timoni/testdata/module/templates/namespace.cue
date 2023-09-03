package templates

#Namespace: {
	_config:    #Config
	apiVersion: "v1"
	kind:       "Namespace"
	metadata: {
		name:      "\(_config.metadata.name)-ns"
		labels:    _config.metadata.labels
		if _config.metadata.annotations != _|_ {
			annotations: _config.metadata.annotations
		}
	}
}
