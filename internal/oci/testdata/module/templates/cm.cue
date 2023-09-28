package templates

import "strings"

#ConfigMap: {
	_config:    #Config
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata:   _config.metadata
	data: hostname: _config.hostname
}
