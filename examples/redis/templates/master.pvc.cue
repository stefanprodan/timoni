package templates

import (
	corev1 "k8s.io/api/core/v1"
	timoniv1 "timoni.sh/core/v1alpha1"
)

#MasterPVC: corev1.#PersistentVolumeClaim & {
	#config:    #Config
	apiVersion: "v1"
	kind:       "PersistentVolumeClaim"
	metadata: timoniv1.#MetaComponent & {
		#Meta:      #config.metadata
		#Component: "master"
	}
	spec: corev1.#PersistentVolumeClaimSpec & {
		storageClassName: #config.persistence.storageClass
		resources: requests: storage: #config.persistence.size
		accessModes: ["ReadWriteOnce"]
	}
}
