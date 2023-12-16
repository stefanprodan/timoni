package templates

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	timoniv1 "timoni.sh/core/v1alpha1"
)

#MasterDeployment: appsv1.#Deployment & {
	_config: #Config
	_selectorLabel: {
		"\(timoniv1.#StdLabelName)": "\(_config.metadata.name)-master"
	}
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: timoniv1.#MetaComponent & {
		#Meta:      _config.metadata
		#Component: "master"
	}
	spec: appsv1.#DeploymentSpec & {
		strategy: type:        "Recreate"
		selector: matchLabels: _selectorLabel
		template: {
			metadata: {
				labels: _selectorLabel
				if _config.podAnnotations != _|_ {
					annotations: _config.podAnnotations
				}
			}
			spec: corev1.#PodSpec & {
				serviceAccountName: _config.metadata.name
				containers: [
					{
						name:            "redis"
						image:           _config.image.reference
						imagePullPolicy: _config.image.pullPolicy
						ports: [{
							name:          "redis"
							containerPort: 6379
							protocol:      "TCP"
						}]
						command: [
							"redis-server",
							"/redis-master/redis.conf",
							if _config.password != _|_ {
								"--requirepass \(_config.password)"
							},
						]
						livenessProbe: {
							tcpSocket: port: "redis"
							initialDelaySeconds: 2
							timeoutSeconds:      2
						}
						readinessProbe: {
							exec: command: ["redis-cli", "ping"]
							initialDelaySeconds: 2
							timeoutSeconds:      5
						}
						volumeMounts: [
							{
								mountPath: "/data"
								name:      "data"
							},
							{
								mountPath: "/redis-master"
								name:      "config"
							},
						]
						if _config.resources != _|_ {
							resources: _config.resources
						}
						if _config.securityContext != _|_ {
							securityContext: _config.securityContext
						}
					},
				]
				volumes: [
					{
						name: "data"
						if !_config.persistence.enabled {
							emptyDir: {}
						}
						if _config.persistence.enabled {
							persistentVolumeClaim: claimName: "\(_config.metadata.name)-master"
						}
					},
					{
						name: "config"
						configMap: {
							name: "\(_config.metadata.name)"
							items: [{
								key:  "redis.conf"
								path: key
							}]
						}
					},
				]
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
	}
}
