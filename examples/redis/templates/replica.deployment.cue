package templates

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	timoniv1 "timoni.sh/core/v1alpha1"
)

#ReplicaDeployment: appsv1.#Deployment & {
	#config: #Config
	_selectorLabel: {
		"\(timoniv1.#StdLabelName)": "\(#config.metadata.name)-replica"
	}
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: timoniv1.#MetaComponent & {
		#Meta:      #config.metadata
		#Component: "replica"
	}
	spec: appsv1.#DeploymentSpec & {
		strategy: type: "RollingUpdate"
		replicas: #config.readonly.replicas
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
							"--replicaof",
							"\(#config.metadata.name).\(#config.metadata.namespace).svc.\(#config.clusterDomain)",
							"\(#config.service.port)",
							"--include",
							"/redis-replica/redis.conf",
							if #config.password != _|_ {
								"--masterauth \(#config.password)"
							},
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
								mountPath: "/redis-replica"
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
						emptyDir: {}
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
