package templates

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	timoniv1 "timoni.sh/core/v1alpha1"
)

// Config defines the schema and defaults for the Instance values.
#Config: {
	// Metadata (common to all resources)
	metadata: metav1.#ObjectMeta
	metadata: name:      string & =~"^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$" & strings.MaxRunes(63)
	metadata: namespace: string & strings.MaxRunes(63)
	metadata: labels:    *selectorLabels | {[ string]: string}
	metadata: labels: "app.kubernetes.io/version": image.tag
	metadata: annotations?: {[ string]:            string}

	// App settings
	message: string

	// Deployment
	replicas:       *1 | int & >0
	selectorLabels: *{"app.kubernetes.io/name": metadata.name} | {[ string]: string}

	// Pod
	podAnnotations?: {[ string]: string}
	podSecurityContext?: corev1.#PodSecurityContext
	imagePullSecrets?: [...corev1.LocalObjectReference]
	tolerations?: [ ...corev1.#Toleration]
	affinity?: corev1.#Affinity
	topologySpreadConstraints?: [...corev1.#TopologySpreadConstraint]

	// Container
	image:            timoniv1.#Image
	imagePullPolicy:  *"IfNotPresent" | string
	resources?:       corev1.#ResourceRequirements
	securityContext?: corev1.#SecurityContext

	// Service
	service: port: *80 | int & >0 & <=65535
}

// Instance takes the config values and outputs the Kubernetes objects.
#Instance: {
	config: #Config

	objects: {
		sa:  #ServiceAccount & {_config: config}
		svc: #Service & {_config:        config}
		cm:  #ConfigMap & {_config:      config}

		deploy: #Deployment & {
			_config: config
			_cmName: objects.cm.metadata.name
		}
	}
}
