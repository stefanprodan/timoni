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
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/encoding/yaml"
	ssautil "github.com/fluxcd/pkg/ssa/utils"
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

	if err := value.Validate(cue.Concrete(true), cue.Final()); err != nil {
		return nil, err
	}

	iter, err := value.Fields(cue.Concrete(true), cue.Final())
	if err != nil {
		return nil, fmt.Errorf("getting resources failed: %w", err)
	}
	for iter.Next() {
		name := iter.Selector().String()
		expr := iter.Value()
		if expr.Err() != nil {
			return nil, fmt.Errorf("getting value of resource list %q failed: %w", name, expr.Err())
		}

		items, err := expr.List()
		if err != nil {
			return nil, fmt.Errorf("listing objects in resource list %q failed: %w", name, err)
		}

		data, err := yaml.EncodeStream(items)
		if err != nil {
			return nil, fmt.Errorf("converting objects for resource list %q failed: %w", name, err)
		}

		objects, err := ssautil.ReadObjects(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("loading objects for resource list %q failed: %w", name, err)
		}

		sets = append(sets, ResourceSet{
			Name:    name,
			Objects: objects,
		})
	}
	return sets, nil
}
