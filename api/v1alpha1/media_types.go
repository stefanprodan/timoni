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
	// UserAgent is the agent name used for OCI operations.
	UserAgent = "timoni/v1"

	// ConfigMediaType is the OCI media type for the config layer.
	ConfigMediaType = "application/vnd.timoni.config.v1+json"

	// ContentMediaType is the OCI media type for the content layer.
	ContentMediaType = "application/vnd.timoni.content.v1.tar+gzip"
)
