package templates

import (
	corev1 "k8s.io/api/core/v1"
	timoniv1 "timoni.sh/core/v1alpha1"
)

#MasterService: corev1.#Service & {
	#config: #Config
	_selectorLabel: {
		"\(timoniv1.#StdLabelName)": "\(#config.metadata.name)-master"
	}
	apiVersion: "v1"
	kind:       "Service"
	metadata:   #config.metadata
	spec: corev1.#ServiceSpec & {
		type:     corev1.#ServiceTypeClusterIP
		selector: _selectorLabel
		ports: [{
			name:       "redis"
			port:       #config.service.port
			targetPort: "\(name)"
			protocol:   "TCP"
		}]
	}
}
