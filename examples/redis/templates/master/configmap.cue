package master

import (
	corev1 "k8s.io/api/core/v1"
	"timoni.sh/redis/templates/config"
)

#ConfigMap: corev1.#ConfigMap & {
	#config:    config.#Config
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata:   #config.metadata
	data:
		"redis.conf": """
			maxmemory \(#config.maxmemory)mb
			maxmemory-policy allkeys-lru

			dir /data
			save \"\"
			appendonly yes

			protected-mode no
			rename-command CONFIG \"\"

			"""
}
