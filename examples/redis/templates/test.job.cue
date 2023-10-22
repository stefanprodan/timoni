package templates

import (
	"encoding/yaml"
	"uuid"

	corev1 "k8s.io/api/core/v1"
	batchv1 "k8s.io/api/batch/v1"
	timoniv1 "timoni.sh/core/v1alpha1"
)

#TestJob: batchv1.#Job & {
	_config:    #Config
	apiVersion: "batch/v1"
	kind:       "Job"
	metadata: {
		name:        "\(_config.metadata.name)-test"
		namespace:   _config.metadata.namespace
		labels:      _config.metadata.labels
		annotations: timoniv1.Action.Force
	}
	spec: batchv1.#JobSpec & {
		template: corev1.#PodTemplateSpec & {
			metadata: labels: _config.metadata.labels
			let _checksum = uuid.SHA1(uuid.ns.DNS, yaml.Marshal(_config))
			metadata: annotations: "timoni.sh/checksum": "\(_checksum)"
			spec: {
				containers: [{
					name:  "redis-cli"
					image: _config.image.reference
					command: [
						"redis-cli",
						if _config.password != _|_ {
							"-a"
						},
						if _config.password != _|_ {
							"\(_config.password)"
						},
						"-h",
						"\(_config.metadata.name)",
						"-p",
						"\(_config.service.port)",
						"PING",
					]
				}]
				restartPolicy: "Never"
				if _config.podSecurityContext != _|_ {
					securityContext: _config.podSecurityContext
				}
				if _config.topologySpreadConstraints != _|_ {
					topologySpreadConstraints: _config.topologySpreadConstraints
				}
				if _config.affinity != _|_ {
					affinity: _config.affinity
				}
				if _config.tolerations != _|_ {
					tolerations: _config.tolerations
				}
				if _config.imagePullSecrets != _|_ {
					imagePullSecrets: _config.imagePullSecrets
				}
			}
		}
		backoffLimit: 1
	}
}
