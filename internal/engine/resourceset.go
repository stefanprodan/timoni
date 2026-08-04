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
	"k8s.io/apimachinery/pkg/runtime"
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

		objects, err := decodeObjects(items)
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

// decodeObjects converts the CUE values in the given iterator to Kubernetes
// unstructured objects. Values which are not Kubernetes objects are silently
// dropped from the result, and Kubernetes lists are expanded to their items.
func decodeObjects(items cue.Iterator) ([]*unstructured.Unstructured, error) {
	objects := make([]*unstructured.Unstructured, 0)

	for items.Next() {
		item := items.Value()
		if item.Kind() == cue.NullKind {
			continue
		}

		if needsYAMLDecoding(item) {
			objs, err := decodeYAMLObjects(item)
			if err != nil {
				return nil, err
			}
			objects = append(objects, objs...)
			continue
		}

		data, err := item.MarshalJSON()
		if err != nil {
			return nil, err
		}

		obj := &unstructured.Unstructured{}
		if err := obj.UnmarshalJSON(data); err != nil {
			return nil, err
		}

		if obj.IsList() {
			err = obj.EachListItem(func(item runtime.Object) error {
				objects = append(objects, item.(*unstructured.Unstructured))
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}

		if ssautil.IsKubernetesObject(obj) && !ssautil.IsKustomization(obj) {
			objects = append(objects, obj)
		}
	}

	return objects, nil
}

// needsYAMLDecoding reports whether the value contains fields of a kind
// (bytes, floats) whose JSON encoding differs from the YAML round-trip:
// bytes decode from YAML to raw strings instead of base64, and integral
// floats normalize to integers.
func needsYAMLDecoding(value cue.Value) bool {
	var found bool
	value.Walk(func(v cue.Value) bool {
		if found {
			return false
		}
		switch v.Kind() {
		case cue.BytesKind, cue.FloatKind:
			found = true
			return false
		}
		return true
	}, nil)
	return found
}

// decodeYAMLObjects converts the CUE value to Kubernetes unstructured
// objects through a YAML round-trip, preserving the YAML decoding of
// bytes and integral floats.
func decodeYAMLObjects(value cue.Value) ([]*unstructured.Unstructured, error) {
	data, err := yaml.Encode(value)
	if err != nil {
		return nil, err
	}
	return ssautil.ReadObjects(bytes.NewReader(data))
}
