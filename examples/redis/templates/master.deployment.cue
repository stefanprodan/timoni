package templates

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	timoniv1 "timoni.sh/core/v1alpha1"
)

#MasterDeployment: appsv1.#Deployment & {
	#config: #Config
	_selectorLabel: {
		"\(timoniv1.#StdLabelName)": "\(#config.metadata.name)-master"
	}
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: timoniv1.#MetaComponent & {
		#Meta:      #config.metadata
		#Component: "master"
	}
	spec: appsv1.#DeploymentSpec & {
		strategy: type:        "Recreate"
		selector: matchLabels: _selectorLabel
		template: {
			metadata: {
				labels: _selectorLabel
				if #config.podAnnotations != _|_ {
					annotations: #config.podAnnotations
				}
			}
			spec: corev1.#PodSpec & {
				serviceAccountName: #config.metadata.name
				containers: [
					{
						name:            "redis"
						image:           #config.image.reference
						imagePullPolicy: #config.image.pullPolicy
						ports: [{
							name:          "redis"
							containerPort: 6379
							protocol:      "TCP"
						}]
						command: [
							"redis-server",
							"/redis-master/redis.conf",
							if #config.password != _|_ {
								"--requirepass \(#config.password)"
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
						if #config.resources != _|_ {
							resources: #config.resources
						}
						if #config.securityContext != _|_ {
							securityContext: #config.securityContext
						}
					},
				]
				volumes: [
					{
						name: "data"
						if !#config.persistence.enabled {
							emptyDir: {}
						}
						if #config.persistence.enabled {
							persistentVolumeClaim: claimName: "\(#config.metadata.name)-master"
						}
					},
					{
						name: "config"
						configMap: {
							name: "\(#config.metadata.name)"
							items: [{
								key:  "redis.conf"
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
				if #config.affinity != _|_ {
					affinity: #config.affinity
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
