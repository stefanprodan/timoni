package templates

import (
	corev1 "k8s.io/api/core/v1"
)

#MasterPVC: corev1.#PersistentVolumeClaim & {
	_config:    #Config
	apiVersion: "v1"
	kind:       "PersistentVolumeClaim"
	metadata: {
		name:      "\(_config.metadata.name)-master"
		namespace: _config.metadata.namespace
		labels:    _config.metadata.labels
		if _config.metadata.annotations != _|_ {
			annotations: _config.metadata.annotations
		}
	}
	spec: corev1.#PersistentVolumeClaimSpec & {
		storageClassName: _config.persistence.storageClass
		resources: requests: storage: _config.persistence.size
		accessModes: ["ReadWriteOnce"]
	}
}
