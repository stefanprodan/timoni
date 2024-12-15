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

import "cuelang.org/go/cue"

const (
	// BundleAPIVersionSelector is the CUE path for the Timoni's bundle API version.
	BundleAPIVersionSelector Selector = "bundle.apiVersion"

	// BundleName is the CUE path for the Timoni's bundle name.
	BundleName Selector = "bundle.name"

	// BundleInstancesSelector is the CUE path for the Timoni's bundle instances.
	BundleInstancesSelector Selector = "bundle.instances"

	// BundleModuleURLSelector is the CUE path for the Timoni's bundle module url.
	BundleModuleURLSelector Selector = "module.url"

	// BundleModuleVersionSelector is the CUE path for the Timoni's bundle module version.
	BundleModuleVersionSelector Selector = "module.version"

	// BundleModuleDigestSelector is the CUE path for the Timoni's bundle module digest.
	BundleModuleDigestSelector Selector = "module.digest"

	// BundleNamespaceSelector is the CUE path for the Timoni's bundle instance namespace.
	BundleNamespaceSelector Selector = "namespace"

	// BundleValuesSelector is the CUE path for the Timoni's bundle instance values.
	BundleValuesSelector Selector = "values"

	// BundleNameLabelKey is the Kubernetes label key for tracking Timoni's bundle by name.
	BundleNameLabelKey = "bundle.timoni.sh/name"
)

// BundleSchema defines the v1alpha1 CUE schema for Timoni's bundle API.
// TODO: switch to go:embed when this is available https://github.com/cue-lang/cue/issues/607
const BundleSchema = `
import "strings"

#Bundle: {
	apiVersion: string & =~"^v1alpha1$"
	name:       string & =~"^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$" & strings.MaxRunes(63) & strings.MinRunes(1)
	instances: [string & =~"^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$" & strings.MaxRunes(63) & strings.MinRunes(1)]: {
		module: close({
			url:     string & =~"^(oci|file)://.*$"
			version: *"latest" | string
			digest?: string
		})
		namespace: string & =~"^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$" & strings.MaxRunes(63) & strings.MinRunes(1)
		values: {...}
	}
}

bundle: #Bundle
`

// Bundle holds the information about the bundle name and the list of instances.
// +k8s:deepcopy-gen=false
type Bundle struct {
	// Name is the name of the bundle.
	Name string `json:"name"`

	// Instances is a list of instances defined in the bundle.
	Instances []*BundleInstance `json:"instances"`
}

// BundleInstance holds the information about the instance name, namespace, module and values.
// +k8s:deepcopy-gen=false
type BundleInstance struct {
	// Bundle is the name of the bundle this instance belongs to.
	Bundle string `json:"bundle"`

	// Cluster is the name of the cluster this instance belongs to.
	Cluster string `json:"cluster,omitempty"`

	// Name is the name of the instance.
	Name string `json:"name"`

	// Namespace is the namespace where the instance will be installed.
	Namespace string `json:"namespace"`

	// Module is a reference to the module's artifact in the registry.
	Module ModuleReference `json:"module"`

	// Values hold the user-supplied configuration of this instance.
	Values cue.Value `json:"values,omitempty"`
}
