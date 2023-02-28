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

package engine

import (
	"bytes"
	"cuelang.org/go/cue"
	"cuelang.org/go/encoding/yaml"
	"fmt"
	"github.com/fluxcd/pkg/ssa"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ResourceSet is a named list of Kubernetes resource objects.
type ResourceSet struct {

	// Name of the object list.
	Name string `json:"name"`

	// Objects holds the list of Kubernetes objects.
	// +optional
	Objects []*unstructured.Unstructured `json:"objects,omitempty"`
}

// GetResources converts the CUE value to a list of ResourceSets.
func GetResources(value cue.Value) ([]ResourceSet, error) {
	var sets []ResourceSet
	iter, _ := value.Fields(cue.Concrete(true))
	for iter.Next() {
		name := iter.Selector().String()
		expr := iter.Value()
		switch expr.Kind() {
		case cue.ListKind:
			items, err := expr.List()
			if err != nil {
				return nil, fmt.Errorf("listing objects for %s failed, error: %w", name, err)
			}

			data, err := yaml.EncodeStream(items)
			if err != nil {
				return nil, fmt.Errorf("encoding objects for %s failed, error: %w", name, err)
			}

			objects, err := ssa.ReadObjects(bytes.NewReader(data))
			if err != nil {
				return nil, fmt.Errorf("decoding objects for %s failed, error: %w", name, err)
			}

			sets = append(sets, ResourceSet{
				Name:    name,
				Objects: objects,
			})
		default:
			return nil, fmt.Errorf("objects in %s are not of type cue.ListKind, got %v", name, value.Kind())
		}
	}
	return sets, nil
}
