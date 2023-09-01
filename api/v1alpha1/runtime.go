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

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// RuntimePIVersionSelector is the CUE path for the Timoni's runtime API version.
	RuntimePIVersionSelector Selector = "runtime.apiVersion"

	// RuntimeName is the CUE path for the Timoni's bundle name.
	RuntimeName Selector = "runtime.name"

	// RuntimeValuesSelector is the CUE path for the Timoni's runtime values.
	RuntimeValuesSelector Selector = "runtime.values"
)

// RuntimeSchema defines the v1alpha1 CUE schema for Timoni's runtime API.
const RuntimeSchema = `
import "strings"

#RuntimeValue: {
	query: string
	for: {[string & =~"^(([A-Za-z0-9][-A-Za-z0-9_]*)?[A-Za-z0-9])?$" & strings.MaxRunes(63) & strings.MinRunes(1)]: string}
	optional: *false | bool
}

#Runtime: {
	apiVersion: string & =~"^v1alpha1$"
	name:       string & =~"^(([A-Za-z0-9][-A-Za-z0-9_]*)?[A-Za-z0-9])?$" & strings.MaxRunes(63) & strings.MinRunes(1)
	values: [...#RuntimeValue]
}
`

// Runtime holds the list of in-cluster resources and the
// CUE expressions for extracting specific fields values.
type Runtime struct {
	// Name of the runtime.
	Name string `json:"name"`

	// Refs is the list of in-cluster resource references.
	Refs []RuntimeResourceRef `json:"refs"`
}

// RuntimeResourceRef holds the data needed to query the fields
// of a Kubernetes resource using CUE expressions.
type RuntimeResourceRef struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Expressions map[string]string `json:"selector"`
	Optional    bool              `json:"optional"`
}

// RuntimeValue holds the query information for in-cluster values.
type RuntimeValue struct {
	// Query in the format 'k8s:<apiVersion>:<kind>:<namespace>:<name>'.
	Query string `json:"query"`

	// For is a map with key values in the format '<name>: <CUE expression>'.
	For map[string]string `json:"for"`

	// Optional is a flag for ignoring not found resources.
	Optional bool `json:"optional"`
}

// ToResourceRef converts the RuntimeValue to a RuntimeResourceRef by parsing the query data.
func (rv *RuntimeValue) ToResourceRef() (*RuntimeResourceRef, error) {
	parts := strings.Split(rv.Query, ":")

	if parts[0] != "k8s" {
		return nil, fmt.Errorf("faild to parse '%s': query must start with k8s", rv.Query)
	}

	if len(parts) < 4 {
		return nil, fmt.Errorf("faild to parse '%s': invalid number of parts", rv.Query)
	}

	ref := RuntimeResourceRef{
		TypeMeta: metav1.TypeMeta{
			APIVersion: parts[1],
			Kind:       parts[2],
		},
		ObjectMeta:  metav1.ObjectMeta{},
		Expressions: map[string]string{},
		Optional:    rv.Optional,
	}

	if len(parts) == 5 {
		ref.Namespace = parts[3]
		ref.Name = parts[4]
	} else {
		ref.Name = parts[3]
	}

	for k, v := range rv.For {
		ref.Expressions[k] = v
	}

	return &ref, nil
}
