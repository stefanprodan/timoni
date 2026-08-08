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
	"errors"
	"fmt"
	"io"

	"cuelang.org/go/cue"
	"cuelang.org/go/encoding/yaml"
	ssautil "github.com/fluxcd/pkg/ssa/utils"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	apiyaml "k8s.io/apimachinery/pkg/util/yaml"
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
// If objects in multiple resource lists are found invalid, the returned
// error aggregates the failures of all lists, retrievable with errors.Unwrap.
func GetResources(value cue.Value) ([]ResourceSet, error) {
	var sets []ResourceSet
	var errs []error

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
			errs = append(errs, fmt.Errorf("loading objects for resource list %q failed: %w", name, err))
			continue
		}

		sets = append(sets, ResourceSet{
			Name:    name,
			Objects: objects,
		})
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return sets, nil
}

// decodeObjects converts the CUE values in the given iterator to Kubernetes
// unstructured objects. Null values and Kustomize config objects are skipped,
// and Kubernetes lists are expanded to their items. Objects which fail the
// validateObjectMeta checks yield an error naming their CUE path, one per
// violation, aggregated with errors.Join across all items.
func decodeObjects(items cue.Iterator) ([]*unstructured.Unstructured, error) {
	objects := make([]*unstructured.Unstructured, 0)
	var errs []error

	for items.Next() {
		item := items.Value()
		if item.Kind() == cue.NullKind {
			continue
		}

		var (
			objs []*unstructured.Unstructured
			err  error
		)
		if needsYAMLDecoding(item) {
			objs, err = decodeYAMLObjects(item)
		} else {
			objs, err = decodeJSONObjects(item)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("decoding object at path %s failed: %w", item.Path(), err))
			continue
		}

		for _, obj := range objs {
			if ssautil.IsKustomization(obj) {
				continue
			}

			verrs := validateObjectMeta(obj)
			for _, verr := range verrs {
				errs = append(errs, fmt.Errorf("invalid object at path %s: %w", item.Path(), verr))
			}
			if len(verrs) == 0 {
				objects = append(objects, obj)
			}
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return objects, nil
}

// listItems expands a Kubernetes list object to its items.
func listItems(list *unstructured.Unstructured) ([]*unstructured.Unstructured, error) {
	var objects []*unstructured.Unstructured
	err := list.EachListItem(func(item runtime.Object) error {
		objects = append(objects, item.(*unstructured.Unstructured))
		return nil
	})
	return objects, err
}

// decodeJSONObjects converts the CUE value to Kubernetes unstructured
// objects through its JSON encoding, expanding Kubernetes lists to
// their items.
func decodeJSONObjects(value cue.Value) ([]*unstructured.Unstructured, error) {
	data, err := value.MarshalJSON()
	if err != nil {
		return nil, err
	}

	obj := &unstructured.Unstructured{}
	if err := obj.UnmarshalJSON(data); err != nil {
		return nil, err
	}

	if obj.IsList() {
		return listItems(obj)
	}

	return []*unstructured.Unstructured{obj}, nil
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
// bytes and integral floats, and expanding Kubernetes lists to their items.
func decodeYAMLObjects(value cue.Value) ([]*unstructured.Unstructured, error) {
	data, err := yaml.Encode(value)
	if err != nil {
		return nil, err
	}

	reader := apiyaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 2048)
	objects := make([]*unstructured.Unstructured, 0)
	for {
		obj := &unstructured.Unstructured{}
		err := reader.Decode(obj)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}

		if obj.IsList() {
			items, err := listItems(obj)
			if err != nil {
				return nil, err
			}
			objects = append(objects, items...)
			continue
		}

		objects = append(objects, obj)
	}
	return objects, nil
}
