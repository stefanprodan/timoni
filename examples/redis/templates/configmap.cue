package templates

import (
	corev1 "k8s.io/api/core/v1"
)

#ConfigMap: corev1.#ConfigMap & {
	_config:    #Config
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata:   _config.metadata
	data:
		"redis.conf": """
			maxmemory \(_config.maxmemory)mb
			maxmemory-policy allkeys-lru

			dir /data
			save \"\"
			appendonly yes

			protected-mode no
			rename-command CONFIG \"\"

			"""
}
