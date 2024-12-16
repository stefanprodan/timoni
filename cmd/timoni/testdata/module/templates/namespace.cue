package templates

import (
	timoniv1 "timoni.sh/core/v1alpha1"
)

#Namespace: {
	#config:    #Config
	apiVersion: "v1"
	kind:       "Namespace"
	metadata: timoniv1.#MetaClusterComponent & {
		#Meta:      #config.metadata
		#Component: "ns"
	}
}
