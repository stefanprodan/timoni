package templates

import (
	timoniv1 "timoni.sh/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// Config defines the schema and defaults for the Instance values.
#Config: {
	// The kubeVersion is a required field, set at apply-time
	// via timoni.cue by querying the user's Kubernetes API.
	kubeVersion!: string
	// Using the kubeVersion you can enforce a minimum Kubernetes minor version.
	// By default, the minimum Kubernetes version is set to 1.20.
	clusterVersion: timoniv1.#SemVer & {#Version: kubeVersion, #Minimum: "1.20.0"}

	// The moduleVersion is set from the user-supplied module version.
	// This field is used for the `app.kubernetes.io/version` label.
	moduleVersion!: string

	// The Kubernetes metadata common to all resources.
	// The `metadata.name` and `metadata.namespace` fields are
	// set from the user-supplied instance name and namespace.
	metadata: timoniv1.#Metadata & {#Version: moduleVersion}

	// The labels allows adding `metadata.labels` to all resources.
	// The `app.kubernetes.io/name` and `app.kubernetes.io/version` labels
	// are automatically generated and can't be overwritten.
	metadata: labels: timoniv1.#Labels

	// The annotations allows adding `metadata.annotations` to all resources.
	metadata: annotations?: timoniv1.#Annotations

	// Redis config
	maxmemory: *512 | int & >=64
	readonly: replicas: *1 | int & >=0
	persistence: {
		enabled:      *true | bool
		storageClass: *"standard" | string
		size:         *"8Gi" | string
	}
	password?: string & =~"^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$"

	// Container image
	image: timoniv1.#Image
	imagePullSecrets?: [...corev1.LocalObjectReference]

	// Resource requirements
	resources: corev1.#ResourceRequirements
	resources: requests: memory: *"64Mi" | string
	resources: limits: memory:   *"\(maxmemory+32)Mi" | string

	// The resources allows setting the container resource requirements.
	// By default, each Redis container requests 100m CPU and 64Mi memory.
	resources: timoniv1.#ResourceRequirements & {
		requests: {
			cpu:    *"100m" | timoniv1.#CPUQuantity
			memory: *"64Mi" | timoniv1.#MemoryQuantity
		}
		limits: {
			memory: *"\(maxmemory+32)Mi" | timoniv1.#MemoryQuantity
		}
	}

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

	// Pod affinity settings (common to all deployments)
	affinity: corev1.#Affinity
	affinity: nodeAffinity: requiredDuringSchedulingIgnoredDuringExecution: nodeSelectorTerms: [{
		matchExpressions: [{
			key:      "kubernetes.io/os"
			operator: "In"
			values: ["linux"]
		}]
	}]

	// Pod optional settings (common to all deployments)
	podAnnotations?: {[string]: string}
	tolerations?: [...corev1.#Toleration]
	topologySpreadConstraints?: [...corev1.#TopologySpreadConstraint]

	// Service
	service: port: *6379 | int & >0 & <=65535
	clusterDomain: "cluster.local"

	// Test Job disabled by default.
	test: {
		enabled: *false | bool
	}
}

// Instance takes the config values and outputs the Kubernetes objects.
#Instance: {
	config: #Config

	master: objects: {
		"\(config.metadata.name)-sa": #ServiceAccount & {_config: config}
		"\(config.metadata.name)-cm": #ConfigMap & {_config: config}

		if config.persistence.enabled {
			"\(config.metadata.name)-pvc": #MasterPVC & {_config: config}
		}

		"\(config.metadata.name)-svc": #MasterService & {_config: config}
		"\(config.metadata.name)-deploy": #MasterDeployment & {_config: config}
	}

	replica: objects: {
		"\(config.metadata.name)-deploy-replica": #ReplicaDeployment & {_config: config}
		"\(config.metadata.name)-svc-replica": #ReplicaService & {_config: config}
	}

	test: objects: {
		"\(config.metadata.name)-ping-master": #TestJob & {_config: config}
	}
}
