package templates

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	timoniv1 "timoni.sh/core/v1alpha1"
)

#Deployment: appsv1.#Deployment & {
	#config: #Config
	#cmName: string

	// The affinity rules generated from the affinity values;
	// the anti-affinity presets match the instance selector labels.
	_affinity: timoniv1.#Affinity & {
		#Values:      #config.affinity
		#MatchLabels: #config.selector.labels
	}

	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata:   #config.metadata
	spec: appsv1.#DeploymentSpec & {
		replicas: #config.replicas
		selector: matchLabels: #config.selector.labels
		template: {
			metadata: {
				labels: #config.selector.labels
				if #config.podAnnotations != _|_ {
					annotations: #config.podAnnotations
				}
			}
			spec: corev1.#PodSpec & {
				serviceAccountName: #config.metadata.name
				containers: [
					{
						name:            #config.metadata.name
						image:           #config.image.reference
						imagePullPolicy: #config.image.pullPolicy
						ports: [
							{
								name:          "http"
								containerPort: 8080
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
								path: "/healthz"
								port: "http"
							}
						}
						volumeMounts: [
							{
								mountPath: "/etc/nginx/conf.d"
								name:      "config"
							},
							{
								mountPath: "/usr/share/nginx/html"
								name:      "html"
							},
						]
						resources:       #config.resources
						securityContext: #config.securityContext
					},
				]
				volumes: [
					{
						name: "config"
						configMap: {
							name: #cmName
							items: [{
								key:  "nginx.default.conf"
								path: key
							}]
						}
					},
					{
						name: "html"
						configMap: {
							name: #cmName
							items: [{
								key:  "index.html"
								path: key
							}]
						}
					},
				]
				if #config.podSecurityContext != _|_ {
					securityContext: #config.podSecurityContext
				}
				if #config.topologySpreadConstraints != _|_ {
					topologySpreadConstraints: #config.topologySpreadConstraints
				}
				nodeSelector: #config.nodeSelector
				if _affinity.#Enabled {
					affinity: _affinity
				}
				if #config.tolerations != _|_ {
					tolerations: #config.tolerations
				}
				if #config.imagePullSecrets != _|_ {
					imagePullSecrets: #config.imagePullSecrets
				}
			}
		}
	}
}
