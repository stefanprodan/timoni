package templates

import (
	corev1 "k8s.io/api/core/v1"
)

#ReplicaService: corev1.#Service & {
	_config: #Config
	_selectorLabel: "app.kubernetes.io/name": "\(_config.metadata.name)-replica"
	apiVersion: "v1"
	kind:       "Service"
	metadata: {
		name:      "\(_config.metadata.name)-readonly"
		namespace: _config.metadata.namespace
		labels:    _selectorLabel
		labels:    _config.metadata.labels
		if _config.metadata.annotations != _|_ {
			annotations: _config.metadata.annotations
		}
	}
	spec: corev1.#ServiceSpec & {
		type:     corev1.#ServiceTypeClusterIP
		selector: _selectorLabel
		ports: [
			{
				name:       "redis"
				port:       _config.service.port
				targetPort: "\(name)"
				protocol:   "TCP"
			},
		]
	}
}
