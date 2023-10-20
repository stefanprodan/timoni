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

// LatestVersion is the tag name that
// denotes the latest stable version of a module.
const LatestVersion = "latest"

// ModuleReference contains the information necessary to locate
// a module's OCI artifact in the registry.
type ModuleReference struct {
	// Name of the module.
	Name string `json:"name"`

	// Repository is the OCI artifact repo name in the format
	// 'oci://<reg.host>/<org>/<repo>'.
	Repository string `json:"repository"`

	// Version is the OCI artifact tag in strict semver format.
	Version string `json:"version"`

	// Digest of the OCI artifact in the format '<sha-type>:<hex>'.
	Digest string `json:"digest"`
}

// ImageReference contains the information necessary to locate
// a container's OCI artifact in the registry.
type ImageReference struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Digest     string `json:"digest"`
	Reference  string `json:"reference"`
}
