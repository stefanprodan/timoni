package templates

import "strings"

#KubeConfig: {
	_config:    #Config
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata:   _config.metadata
	data: {
		url:           "https://\(_config.hostname)"
		moduleVersion: _config.moduleVersion
		if strings.HasPrefix(_config.kubeVersion, "1.25") {
			kubeVersion: _config.kubeVersion
		}
	}
}
