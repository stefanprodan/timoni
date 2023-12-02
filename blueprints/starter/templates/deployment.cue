package templates

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

#Deployment: appsv1.#Deployment & {
	_config:    #Config
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata:   _config.metadata
	spec: appsv1.#DeploymentSpec & {
		replicas: _config.replicas
		selector: matchLabels: _config.selector.labels
		template: {
			metadata: {
				labels: _config.selector.labels
				if _config.pod.annotations != _|_ {
					annotations: _config.pod.annotations
				}
			}
			spec: corev1.#PodSpec & {
				containers: [
					{
						name:            _config.metadata.name
						image:           _config.image.reference
						imagePullPolicy: _config.image.pullPolicy
						ports: [
							{
								name:          "http"
								containerPort: 80
								protocol:      "TCP"
							},
						]
						readinessProbe: {
							httpGet: {
								path: "/"
								port: "http"
							}
							initialDelaySeconds: 5
							periodSeconds:       10
						}
						livenessProbe: {
							tcpSocket: {
								port: "http"
							}
							initialDelaySeconds: 5
							periodSeconds:       5
						}
						if _config.resources != _|_ {
							resources: _config.resources
						}
						if _config.securityContext != _|_ {
							securityContext: _config.securityContext
						}
					},
				]
				if _config.pod.affinity != _|_ {
					affinity: _config.pod.affinity
				}
				if _config.pod.imagePullSecrets != _|_ {
					imagePullSecrets: _config.pod.imagePullSecrets
				}
			}
		}
	}
}
