package templates

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
)

// Config defines the schema and defaults for the Instance values.
#Config: {
	// Redis config
	maxmemory: *512 | int & >=64
	readonly: replicas: *1 | int & >=0
	persistence: {
		enabled:      *true | bool
		storageClass: *"standard" | string
		size:         *"8Gi" | string
	}

	// Metadata (common to all resources)
	metadata: metav1.#ObjectMeta
	metadata: name:      *"redis" | string
	metadata: namespace: *"default" | string
	metadata: labels: {
		"app.kubernetes.io/version": image.tag
		"app.kubernetes.io/part-of": metadata.name
	}
	metadata: annotations?: {[ string]: string}

	// Container image
	image: {
		repository: *"cgr.dev/chainguard/redis" | string
		tag:        *"7.0.8" | string
		pullPolicy: *"IfNotPresent" | string
	}
	imagePullSecrets?: [...corev1.LocalObjectReference]

	// Resource requirements
	resources: corev1.#ResourceRequirements
	resources: requests: memory: *"64Mi" | string
	resources: limits: memory:   *"\(maxmemory+32)Mi" | string

	// Security (common to all deployments)
	podSecurityContext: *{
		fsGroup:    1001
		runAsUser:  1001
		runAsGroup: 1001
	} | corev1.#PodSecurityContext
	securityContext: *{
		allowPrivilegeEscalation: false
		readOnlyRootFilesystem:   true
		runAsNonRoot:             true
		capabilities: drop: ["ALL"]
		seccompProfile: type: "RuntimeDefault"
	} | corev1.#SecurityContext

	// Pod optional settings (common to all deployments)
	podAnnotations?: {[ string]: string}
	tolerations?: [ ...corev1.#Toleration]
	affinity?: corev1.#Affinity
	topologySpreadConstraints?: [...corev1.#TopologySpreadConstraint]

	// Service
	service: port: *6379 | int & >0 & <=65535
	clusterDomain: "cluster.local"
}

// Instance takes the config values and outputs the Kubernetes objects.
#Instance: {
	config: #Config

	master: objects: {
		"\(config.metadata.name)-sa": #ServiceAccount & {_config: config}
		"\(config.metadata.name)-cm": #ConfigMap & {_config:      config}

		if config.persistence.enabled {
			"\(config.metadata.name)-pvc": #MasterPVC & {_config: config}
		}

		"\(config.metadata.name)-svc":    #MasterService & {_config:    config}
		"\(config.metadata.name)-deploy": #MasterDeployment & {_config: config}
	}

	replica: objects: {
		"\(config.metadata.name)-deploy-replica": #ReplicaDeployment & {_config: config}
		"\(config.metadata.name)-svc-replica":    #ReplicaService & {_config:    config}
	}
}
