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

const (
	// BundleAPIVersionSelector is the CUE path for the Timoni's bundle API version.
	BundleAPIVersionSelector Selector = "bundle.apiVersion"

	// BundleInstancesSelector is the CUE path for the Timoni's bundle instances.
	BundleInstancesSelector Selector = "bundle.instances"

	// BundleModuleURLSelector is the CUE path for the Timoni's bundle module url.
	BundleModuleURLSelector Selector = "module.url"

	// BundleModuleVersionSelector is the CUE path for the Timoni's bundle module version.
	BundleModuleVersionSelector Selector = "module.version"

	// BundleNamespaceSelector is the CUE path for the Timoni's bundle instance namespace.
	BundleNamespaceSelector Selector = "namespace"

	// BundleValuesSelector is the CUE path for the Timoni's bundle instance values.
	BundleValuesSelector Selector = "values"
)

// BundleSchema defines the v1alpha1 CUE schema for Timoni's bundle API.
// TODO: switch to go:embed when this is available https://github.com/cue-lang/cue/issues/607
const BundleSchema = `
import "strings"

#Bundle: {
	apiVersion: string & =~"^v1alpha1$"
	instances: [string & =~"^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$" & strings.MaxRunes(63) & strings.MinRunes(1)]: {
		module: close({
			url:     string & =~"^oci://.*$"
			version: string & strings.MinRunes(3)
		})
		namespace: string & =~"^(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?$" & strings.MaxRunes(63) & strings.MinRunes(1)
		values: {...}
	}
}

bundle: #Bundle
`
