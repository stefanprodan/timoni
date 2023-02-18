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

package runtime

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sort"

	"github.com/fluxcd/pkg/ssa"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/object"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// InstanceManager performs operations on the instance's inventory.
type InstanceManager struct {
	Instance apiv1.Instance
}

// NewInstanceManager creates an InstanceManager for the given module.
func NewInstanceManager(name, namespace, values string, moduleRef apiv1.ModuleReference) *InstanceManager {
	inst := apiv1.Instance{
		TypeMeta: metav1.TypeMeta{
			Kind:       apiv1.InstanceKind,
			APIVersion: apiv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Module: moduleRef,
		Values: values,
	}
	return &InstanceManager{Instance: inst}
}

// AddObjects extracts the metadata from the given objects and adds it to the instance inventory.
func (m *InstanceManager) AddObjects(objects []*unstructured.Unstructured) error {
	var entries []apiv1.ResourceRef
	sort.Sort(ssa.SortableUnstructureds(objects))
	for _, om := range objects {
		objMetadata := object.UnstructuredToObjMetadata(om)
		gv, err := schema.ParseGroupVersion(om.GetAPIVersion())
		if err != nil {
			return err
		}
		entries = append(entries, apiv1.ResourceRef{
			ID:      objMetadata.String(),
			Version: gv.Version,
		})
	}

	if m.Instance.Inventory == nil {
		m.Instance.Inventory = &apiv1.ResourceInventory{Entries: entries}
	} else {
		return fmt.Errorf("inventory already containts objects: %v", m.Instance.Inventory)
	}

	return nil
}

// VersionOf returns the API version of the given object if found in this instance.
func (m *InstanceManager) VersionOf(objMetadata object.ObjMetadata) string {
	if inv := m.Instance.Inventory; inv != nil {
		for _, entry := range inv.Entries {
			if entry.ID == objMetadata.String() {
				return entry.Version
			}
		}
	}
	return ""
}

// ListObjects returns the inventory entries as unstructured.Unstructured objects.
func (m *InstanceManager) ListObjects() ([]*unstructured.Unstructured, error) {
	objects := make([]*unstructured.Unstructured, 0)
	if inv := m.Instance.Inventory; inv != nil {
		for _, entry := range inv.Entries {
			objMetadata, err := object.ParseObjMetadata(entry.ID)
			if err != nil {
				return nil, err
			}

			u := &unstructured.Unstructured{}
			u.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   objMetadata.GroupKind.Group,
				Kind:    objMetadata.GroupKind.Kind,
				Version: entry.Version,
			})
			u.SetName(objMetadata.Name)
			u.SetNamespace(objMetadata.Namespace)
			objects = append(objects, u)
		}
	}

	sort.Sort(ssa.SortableUnstructureds(objects))
	return objects, nil
}

// ListMeta returns the inventory entries as object.ObjMetadata objects.
func (m *InstanceManager) ListMeta() (object.ObjMetadataSet, error) {
	var metas []object.ObjMetadata
	if inv := m.Instance.Inventory; inv != nil {
		for _, e := range inv.Entries {
			m, err := object.ParseObjMetadata(e.ID)
			if err != nil {
				return metas, err
			}
			metas = append(metas, m)
		}
	}
	return metas, nil
}

// Diff returns the slice of objects that do not exist in the target inventory.
func (m *InstanceManager) Diff(target *apiv1.ResourceInventory) ([]*unstructured.Unstructured, error) {
	objects := make([]*unstructured.Unstructured, 0)
	if m.Instance.Inventory == nil || target == nil {
		return objects, nil
	}

	aList, err := m.ListMeta()
	if err != nil {
		return nil, err
	}

	tm := InstanceManager{Instance: apiv1.Instance{Inventory: target}}
	bList, err := tm.ListMeta()
	if err != nil {
		return nil, err
	}

	list := aList.Diff(bList)
	if len(list) == 0 {
		return objects, nil
	}

	for _, metadata := range list {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   metadata.GroupKind.Group,
			Kind:    metadata.GroupKind.Kind,
			Version: m.VersionOf(metadata),
		})
		u.SetName(metadata.Name)
		u.SetNamespace(metadata.Namespace)
		objects = append(objects, u)
	}

	sort.Sort(ssa.SortableUnstructureds(objects))
	return objects, nil
}
