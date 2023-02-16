package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Instance holds the information about the module, values
// and the list of the managed Kubernetes resources.
type Instance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Module is a reference to the module's artifact in the registry.
	Module ModuleReference `json:"module"`

	// Values is the module configuration.
	// +optional
	Values string `json:"values,omitempty"`

	// LastTransitionTime is the timestamp (UTC RFC3339) of the last inventory change.
	// +optional
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`

	// Inventory contains the list of Kubernetes resource object references.
	// +optional
	Inventory *ResourceInventory `json:"inventory,omitempty"`
}
