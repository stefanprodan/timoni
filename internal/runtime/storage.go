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
	"context"
	"fmt"
	"github.com/fluxcd/pkg/ssa"
	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

var (
	storagePrefix     = fmt.Sprintf("%s.", apiv1.FieldManager)
	storageDataKey    = strings.ToLower(apiv1.InstanceKind)
	nameLabelKey      = "app.kubernetes.io/name"
	componentLabelKey = "app.kubernetes.io/component"
	createdByLabelKey = "app.kubernetes.io/created-by"
)

// StorageManager manages the inventory in-cluster storage.
type StorageManager struct {
	resManager *ssa.ResourceManager
}

// NewStorageManager creates a storage manager for the given cluster.
func NewStorageManager(resManager *ssa.ResourceManager) *StorageManager {
	return &StorageManager{
		resManager: resManager,
	}
}

// Apply creates or updates the storage object for the given instance.
func (s *StorageManager) Apply(ctx context.Context, i *apiv1.Instance, createNamespace bool) error {
	i.LastTransitionTime = time.Now().UTC().Format(time.RFC3339)
	inst, err := json.Marshal(i)
	if err != nil {
		return err
	}

	if createNamespace {
		if err := s.createNamespace(ctx, i.Namespace); err != nil {
			return err
		}
	}

	cm := s.newSecret(i.Name, i.Namespace)
	cm.Data = map[string][]byte{
		storageDataKey: inst,
	}

	opts := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner(ownerRef.Field),
	}
	return s.resManager.Client().Patch(ctx, cm, client.Apply, opts...)
}

// Get retrieves the instance from the storage.
func (s *StorageManager) Get(ctx context.Context, name, namespace string) (*apiv1.Instance, error) {
	cm := s.newSecret(name, namespace)

	cmKey := client.ObjectKeyFromObject(cm)
	err := s.resManager.Client().Get(ctx, cmKey, cm)
	if err != nil {
		return nil, err
	}

	if _, ok := cm.Data[storageDataKey]; !ok {
		return nil, fmt.Errorf("instance data not found in Secret/%s", cmKey)
	}

	var inst apiv1.Instance
	err = json.Unmarshal(cm.Data[storageDataKey], &inst)
	if err != nil {
		return nil, err
	}

	return &inst, nil
}

// List returns the instances found in the given namespace.
func (s *StorageManager) List(ctx context.Context, namespace string) ([]*apiv1.Instance, error) {
	var res []*apiv1.Instance
	cmList := &corev1.SecretList{}
	err := s.resManager.Client().List(ctx, cmList, client.InNamespace(namespace), s.getOwnerLabels())
	if err != nil {
		return res, err
	}

	for _, cm := range cmList.Items {
		if _, ok := cm.Data[storageDataKey]; !ok {
			return res, fmt.Errorf("instance data not found in Secret/%s/%s",
				cm.GetNamespace(), cm.GetName())
		}

		var inst apiv1.Instance
		err = json.Unmarshal(cm.Data[storageDataKey], &inst)
		if err != nil {
			return res, fmt.Errorf("invalid instance found in Secret/%s/%s, error: %w",
				cm.GetNamespace(), cm.GetName(), err)
		}
		res = append(res, &inst)
	}

	return res, nil
}

// Delete removes the storage for the given instance name and namespace.
func (s *StorageManager) Delete(ctx context.Context, name, namespace string) error {
	cm := s.newSecret(name, namespace)

	cmKey := client.ObjectKeyFromObject(cm)
	err := s.resManager.Client().Delete(ctx, cm)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete Secret/%s, error: %w", cmKey, err)
	}
	return nil
}

// GetStaleObjects returns the list of objects metadata subject to pruning.
func (s *StorageManager) GetStaleObjects(ctx context.Context, i *apiv1.Instance) ([]*unstructured.Unstructured, error) {
	objects := make([]*unstructured.Unstructured, 0)
	existingInst, err := s.Get(ctx, i.Name, i.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return objects, nil
		}
		return nil, err
	}

	tm := InstanceManager{Instance: *existingInst}
	objects, err = tm.Diff(i.Inventory)
	if err != nil {
		return nil, err
	}

	return objects, nil
}

// NamespaceExists returns false if the namespace is not found.
func (s *StorageManager) NamespaceExists(ctx context.Context, name string) (bool, error) {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	if err := s.resManager.Client().Get(ctx, client.ObjectKeyFromObject(ns), ns); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		} else {
			return false, err
		}
	}

	return true, nil
}

// getOwnerLabels returns a label selector matching the storage owner.
func (s *StorageManager) getOwnerLabels() client.MatchingLabels {
	return client.MatchingLabels{
		componentLabelKey: strings.ToLower(apiv1.InstanceKind),
		createdByLabelKey: ownerRef.Field,
	}
}

func (s *StorageManager) newSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      storagePrefix + name,
			Namespace: namespace,
			Labels: map[string]string{
				nameLabelKey:      name,
				componentLabelKey: strings.ToLower(apiv1.InstanceKind),
				createdByLabelKey: ownerRef.Field,
			},
		},
	}
}

// createNamespace creates the inventory namespace if not present.
func (s *StorageManager) createNamespace(ctx context.Context, name string) error {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				createdByLabelKey: ownerRef.Field,
			},
		},
	}

	if err := s.resManager.Client().Get(ctx, client.ObjectKeyFromObject(ns), ns); err != nil {
		if apierrors.IsNotFound(err) {
			opts := []client.PatchOption{
				client.ForceOwnership,
				client.FieldOwner(ownerRef.Field),
			}
			return s.resManager.Client().Patch(ctx, ns, client.Apply, opts...)
		} else {
			return err
		}
	}

	return nil
}
