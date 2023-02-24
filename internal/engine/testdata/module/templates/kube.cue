package templates

#KubeConfig: {
	_config:    #Config
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata:   _config.metadata
	data: {
		url: "https://\(_config.hostname)"
	}
}
