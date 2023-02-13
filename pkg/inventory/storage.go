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
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fluxcd/pkg/ssa"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	KindName          = "inventory"
	storagePrefix     = "timoni."
	nameLabelKey      = "app.kubernetes.io/name"
	componentLabelKey = "app.kubernetes.io/component"
	createdByLabelKey = "app.kubernetes.io/created-by"
)

// Storage manages the Inventory in-cluster storage.
type Storage struct {
	Manager *ssa.ResourceManager
	Owner   ssa.Owner
}

// ApplyInventory creates or updates the storage object for the given inventory.
func (s *Storage) ApplyInventory(ctx context.Context, i *Inventory, createNamespace bool) error {
	resources, err := json.Marshal(i.Resources)
	if err != nil {
		return err
	}

	if createNamespace {
		if err := s.createNamespace(ctx, i.Namespace); err != nil {
			return err
		}
	}

	cm := s.newConfigMap(i.Name, i.Namespace)
	cm.Annotations = s.metaToAnnotations(i)

	cm.Data = map[string]string{
		"resources": string(resources),
	}

	if len(i.Artifacts) > 0 {
		artifacts, err := json.Marshal(i.Artifacts)
		if err != nil {
			return err
		}
		cm.Data["artifacts"] = string(artifacts)
	}

	opts := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner(s.Owner.Field),
	}
	return s.Manager.Client().Patch(ctx, cm, client.Apply, opts...)
}

// GetInventory retrieves the entries from the storage for the given inventory name and namespace.
func (s *Storage) GetInventory(ctx context.Context, i *Inventory) error {
	cm := s.newConfigMap(i.Name, i.Namespace)

	cmKey := client.ObjectKeyFromObject(cm)
	err := s.Manager.Client().Get(ctx, cmKey, cm)
	if err != nil {
		return err
	}

	s.metaFromAnnotations(i, cm.GetAnnotations())

	if _, ok := cm.Data["resources"]; !ok {
		return fmt.Errorf("inventory data not found in ConfigMap/%s", cmKey)
	}
	var entries []Resource
	err = json.Unmarshal([]byte(cm.Data["resources"]), &entries)
	if err != nil {
		return err
	}
	i.Resources = entries

	if artifacts, ok := cm.Data["artifacts"]; ok {
		var list []string
		err = json.Unmarshal([]byte(artifacts), &list)
		if err != nil {
			return err
		}
		i.Artifacts = list
	}

	return nil
}

// ListInventories returns the inventories in the given namespace.
func (s *Storage) ListInventories(ctx context.Context, namespace string) ([]*Inventory, error) {
	var inventories []*Inventory
	cmList := &corev1.ConfigMapList{}
	err := s.Manager.Client().List(ctx, cmList, client.InNamespace(namespace), s.getOwnerLabels())
	if err != nil {
		return inventories, err
	}

	for _, cm := range cmList.Items {
		i := NewInventory(strings.TrimPrefix(cm.GetName(), storagePrefix), cm.GetNamespace())
		if err := s.GetInventory(ctx, i); err != nil {
			return inventories, err
		}
		inventories = append(inventories, i)
	}

	return inventories, nil
}

// DeleteInventory removes the storage for the given inventory name and namespace.
func (s *Storage) DeleteInventory(ctx context.Context, i *Inventory) error {
	cm := s.newConfigMap(i.Name, i.Namespace)

	cmKey := client.ObjectKeyFromObject(cm)
	err := s.Manager.Client().Delete(ctx, cm)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ConfigMap/%s, error: %w", cmKey, err)
	}
	return nil
}

// GetInventoryStaleObjects returns the list of objects metadata subject to pruning.
func (s *Storage) GetInventoryStaleObjects(ctx context.Context, i *Inventory) ([]*unstructured.Unstructured, error) {
	objects := make([]*unstructured.Unstructured, 0)
	existingInventory := NewInventory(i.Name, i.Namespace)
	if err := s.GetInventory(ctx, existingInventory); err != nil {
		if apierrors.IsNotFound(err) {
			return objects, nil
		}
		return nil, err
	}

	objects, err := existingInventory.Diff(i)
	if err != nil {
		return nil, err
	}

	return objects, nil
}

func (s *Storage) getOwnerLabels() client.MatchingLabels {
	return client.MatchingLabels{
		componentLabelKey: KindName,
		createdByLabelKey: s.Owner.Field,
	}
}

func (s *Storage) metaToAnnotations(inv *Inventory) map[string]string {
	annotations := map[string]string{
		s.Owner.Group + "/last-applied-time": time.Now().UTC().Format(time.RFC3339),
	}
	if inv.Source != "" {
		annotations[s.Owner.Group+"/source"] = inv.Source
	}
	if inv.Revision != "" {
		annotations[s.Owner.Group+"/revision"] = inv.Revision
	}

	return annotations
}

func (s *Storage) metaFromAnnotations(inv *Inventory, annotations map[string]string) {
	for k, v := range annotations {
		switch k {
		case s.Owner.Group + "/source":
			inv.Source = v
		case s.Owner.Group + "/revision":
			inv.Revision = v
		case s.Owner.Group + "/last-applied-time":
			inv.LastAppliedAt = v
		}
	}
}

func (s *Storage) newConfigMap(name, namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      storagePrefix + name,
			Namespace: namespace,
			Labels: map[string]string{
				nameLabelKey:      name,
				componentLabelKey: KindName,
				createdByLabelKey: s.Owner.Field,
			},
		},
	}
}

// createNamespace creates the inventory namespace if not present.
func (s *Storage) createNamespace(ctx context.Context, name string) error {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				createdByLabelKey: s.Owner.Field,
			},
		},
	}

	if err := s.Manager.Client().Get(ctx, client.ObjectKeyFromObject(ns), ns); err != nil {
		if apierrors.IsNotFound(err) {
			opts := []client.PatchOption{
				client.ForceOwnership,
				client.FieldOwner(s.Owner.Field),
			}
			return s.Manager.Client().Patch(ctx, ns, client.Apply, opts...)
		} else {
			return err
		}
	}

	return nil
}
