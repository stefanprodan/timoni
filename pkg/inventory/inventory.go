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

package inventory

import (
	"sort"

	"github.com/fluxcd/pkg/ssa"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/object"
)

// Inventory is a record of objects that are applied on a cluster stored as a configmap.
type Inventory struct {
	// Name of the inventory.
	Name string `json:"name"`

	// Namespace of the inventory.
	Namespace string `json:"namespace"`

	// Source is the repository URL.
	Source string `json:"source,omitempty"`

	// Revision is the source revision identifier.
	Revision string `json:"revision,omitempty"`

	// LastAppliedAt is the timestamp (UTC RFC3339) of the last successful apply.
	LastAppliedAt string `json:"lastAppliedTime,omitempty"`

	// Resources is the list of Kubernetes object IDs.
	Resources []Resource `json:"resources"`

	// Artifacts is the list of the OCI URLs.
	Artifacts []string `json:"artifacts"`
}

// Resource contains the information necessary to locate the Kubernetes object.
type Resource struct {
	// ObjectID is the string representation of object.ObjMetadata,
	// in the format '<namespace>_<name>_<group>_<kind>'.
	ObjectID string `json:"id"`

	// ObjectVersion is the API version of this entry kind.
	ObjectVersion string `json:"ver"`
}

func NewInventory(name, namespace string) *Inventory {
	return &Inventory{
		Name:      name,
		Namespace: namespace,
		Resources: []Resource{},
	}
}

// SetSource sets the source url and revision for this inventory.
func (inv *Inventory) SetSource(url, revision string, artifacts []string) {
	inv.Source = url
	inv.Revision = revision
	inv.Artifacts = artifacts
}

// AddObjects extracts the metadata from the given objects and adds it to the inventory.
func (inv *Inventory) AddObjects(objects []*unstructured.Unstructured) error {
	sort.Sort(ssa.SortableUnstructureds(objects))
	for _, om := range objects {
		objMetadata := object.UnstructuredToObjMetadata(om)
		gv, err := schema.ParseGroupVersion(om.GetAPIVersion())
		if err != nil {
			return err
		}

		inv.Resources = append(inv.Resources, Resource{
			ObjectID:      objMetadata.String(),
			ObjectVersion: gv.Version,
		})
	}

	return nil
}

// VersionOf returns the API version of the given object if found in this inventory.
func (inv *Inventory) VersionOf(objMetadata object.ObjMetadata) string {
	for _, entry := range inv.Resources {
		if entry.ObjectID == objMetadata.String() {
			return entry.ObjectVersion
		}
	}
	return ""
}

// ListObjects returns the inventory entries as unstructured.Unstructured objects.
func (inv *Inventory) ListObjects() ([]*unstructured.Unstructured, error) {
	objects := make([]*unstructured.Unstructured, 0)

	for _, entry := range inv.Resources {
		objMetadata, err := object.ParseObjMetadata(entry.ObjectID)
		if err != nil {
			return nil, err
		}

		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   objMetadata.GroupKind.Group,
			Kind:    objMetadata.GroupKind.Kind,
			Version: entry.ObjectVersion,
		})
		u.SetName(objMetadata.Name)
		u.SetNamespace(objMetadata.Namespace)
		objects = append(objects, u)
	}

	sort.Sort(ssa.SortableUnstructureds(objects))
	return objects, nil
}

// ListMeta returns the inventory entries as object.ObjMetadata objects.
func (inv *Inventory) ListMeta() (object.ObjMetadataSet, error) {
	var metas []object.ObjMetadata
	for _, e := range inv.Resources {
		m, err := object.ParseObjMetadata(e.ObjectID)
		if err != nil {
			return metas, err
		}
		metas = append(metas, m)
	}

	return metas, nil
}

// Diff returns the slice of objects that do not exist in the target inventory.
func (inv *Inventory) Diff(target *Inventory) ([]*unstructured.Unstructured, error) {
	objects := make([]*unstructured.Unstructured, 0)
	aList, err := inv.ListMeta()
	if err != nil {
		return nil, err
	}

	bList, err := target.ListMeta()
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
			Version: inv.VersionOf(metadata),
		})
		u.SetName(metadata.Name)
		u.SetNamespace(metadata.Namespace)
		objects = append(objects, u)
	}

	sort.Sort(ssa.SortableUnstructureds(objects))
	return objects, nil
}
