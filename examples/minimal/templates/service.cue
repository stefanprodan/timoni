package templates

import (
	corev1 "k8s.io/api/core/v1"
)

#Service: corev1.#Service & {
	_config:    #Config
	apiVersion: "v1"
	kind:       "Service"
	metadata:   _config.metadata
	spec:       corev1.#ServiceSpec & {
		type:     corev1.#ServiceTypeClusterIP
		selector: _config.selector.labels
		ports: [
			{
				port:       _config.service.port
				protocol:   "TCP"
				name:       "http"
				targetPort: name
			},
		]
	}
}
