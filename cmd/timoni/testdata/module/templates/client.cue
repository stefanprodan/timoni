package templates

import (
	timoniv1 "timoni.sh/core/v1alpha1"
)

#ClientConfig: {
	#config:    #Config
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: timoniv1.#MetaComponent & {
		#Meta:      #config.metadata
		#Component: "client"
	}
	data: {
		server: "tcp://\(#config.domain):9090"
	}
}
