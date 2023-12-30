package master

import (
	corev1 "k8s.io/api/core/v1"
	"timoni.sh/redis/templates/config"
)

#ServiceAccount: corev1.#ServiceAccount & {
	#config:    config.#Config
	apiVersion: "v1"
	kind:       "ServiceAccount"
	metadata:   #config.metadata
}
