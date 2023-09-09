/*
Copyright 2023 Stefan Prodan

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	// APIVersionSelector is the CUE path for the Timoni's API version.
	APIVersionSelector Selector = "timoni.apiVersion"

	// ApplySelector is the CUE path for the Timoni's apply resource sets.
	ApplySelector Selector = "timoni.apply"

	// ValuesSelector is the CUE path for the Timoni's module values.
	ValuesSelector Selector = "values"
)

// InstanceSchema defines the v1alpha1 CUE schema for Timoni's instance API.
const InstanceSchema = `
#Timoni: {
	apiVersion: string & =~"^v1alpha1$"
	instance: {...}
	apply: [string]: [...]
	kubeMinorVersion?: int
}

timoni: #Timoni
`

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
