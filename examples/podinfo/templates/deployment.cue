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
	spec:       appsv1.#DeploymentSpec & {
		if !_config.autoscaling.enabled {
			replicas: _config.replicas
		}
		selector: matchLabels: _config.selectorLabels
		template: {
			metadata: {
				labels: _config.selectorLabels
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
						ports: [
							{
								name:          "http"
								containerPort: 9898
								protocol:      "TCP"
							},
						]
						livenessProbe: {
							httpGet: {
								path: "/healthz"
								port: "http"
							}
						}
						readinessProbe: {
							httpGet: {
								path: "/readyz"
								port: "http"
							}
						}
						if _config.resources != _|_ {
							resources: _config.resources
						}
						if _config.securityContext != _|_ {
							securityContext: _config.securityContext
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
