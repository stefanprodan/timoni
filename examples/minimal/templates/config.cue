package templates

import (
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	timoniv1 "timoni.sh/core/v1alpha1"
)

// Config defines the schema and defaults for the Instance values.
#Config: {
	// The kubeVersion is a required field, set at apply-time
	// via timoni.cue by querying the user's Kubernetes API.
	kubeVersion!: string
	// Using the kubeVersion you can enforce a minimum Kubernetes minor version.
	_kubeMinorVersion: int & >=20
	_kubeMinorVersion: strconv.Atoi(strings.Split(kubeVersion, ".")[1])

	// The moduleVersion is a required field, set at apply-time
	// via timoni.cue from the user-supplied module version.
	// This field is used to set the `app.kubernetes.io/version` label.
	moduleVersion!: string

	// Metadata is for setting custom annotations and labels to all resources.
	// The `metadata.name` and `metadata.namespace` are required fields, set
	// at apply-time via timoni.cue from the user-supplied instance name and namespace.
	// Note that the `app.kubernetes.io/name` and `app.kubernetes.io/version` labels
	// are automatically generated and can't be overwritten.
	metadata: timoniv1.#Metadata & {#Version: moduleVersion}

	// Selector is for setting label selectors in Deployments and Services.
	// The `app.kubernetes.io/name` label selector is automatically generated
	// from the instance name and can't be overwritten.
	selector: timoniv1.#Selector & {#Name: metadata.name}

	// App settings
	message!: string

	// Deployment
	replicas: *1 | int & >0

	// Pod
	podAnnotations?: {[string]: string}
	podSecurityContext?: corev1.#PodSecurityContext
	imagePullSecrets?: [...corev1.LocalObjectReference]
	tolerations?: [...corev1.#Toleration]
	affinity?: corev1.#Affinity
	topologySpreadConstraints?: [...corev1.#TopologySpreadConstraint]

	// Container
	image!:           timoniv1.#Image
	imagePullPolicy:  *"IfNotPresent" | string
	resources?:       corev1.#ResourceRequirements
	securityContext?: corev1.#SecurityContext

	// Service
	service: port: *80 | int & >0 & <=65535

	// Test Job
	test: {
		enabled: *false | bool
		image!:  timoniv1.#Image
	}
}

// Instance takes the config values and outputs the Kubernetes objects.
#Instance: {
	config: #Config

	objects: {
		sa: #ServiceAccount & {_config: config}
		svc: #Service & {_config: config}
		cm: #ConfigMap & {_config: config}

		deploy: #Deployment & {
			_config: config
			_cmName: objects.cm.metadata.name
		}
	}

	tests: {
		"test-svc": #TestJob & {_config: config}
	}
}
