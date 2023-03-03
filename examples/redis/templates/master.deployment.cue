package templates

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

#MasterDeployment: appsv1.#Deployment & {
	_config: #Config
	_name:   "\(_config.metadata.name)-master"
	_selectorLabel: "app.kubernetes.io/name": _name
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {
		name:      _name
		namespace: _config.metadata.namespace
		labels:    _selectorLabel
		labels:    _config.metadata.labels
		if _config.metadata.annotations != _|_ {
			annotations: _config.metadata.annotations
		}
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
						name:            _config.metadata.name
						image:           "\(_config.image.repository):\(_config.image.tag)"
						imagePullPolicy: _config.image.pullPolicy
						ports: [{
							name:          "redis"
							containerPort: 6379
							protocol:      "TCP"
						}]
						command: ["redis-server", "/redis-master/redis.conf"]
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
							persistentVolumeClaim: claimName: _name
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
