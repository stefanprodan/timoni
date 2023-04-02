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

// Selector is an enumeration of the supported CUE paths known to Timoni.
type Selector string

// String returns the string representation of the Selector.
func (s Selector) String() string {
	return string(s)
}

const (
	// APIVersionSelector is the CUE path for the Timoni's API version.
	APIVersionSelector Selector = "timoni.apiVersion"

	// ValuesSelector is the CUE path for the Timoni's module values.
	ValuesSelector Selector = "values"

	// ApplySelector is the CUE path for the Timoni's apply resource sets.
	ApplySelector Selector = "timoni.apply"
)
