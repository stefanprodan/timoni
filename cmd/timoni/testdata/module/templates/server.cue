package templates

import (
	timoniv1 "timoni.sh/core/v1alpha1"
)

#ServerConfig: {
	#config:    #Config
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: timoniv1.#MetaComponent & {
		#Meta:      #config.metadata
		#Component: "server"
	}
	data: {
		hostname: "\(#config.domain)"
		port:     "9090"
	}
}
