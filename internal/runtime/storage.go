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
	"sort"
	"strings"
	"time"

	"github.com/fluxcd/pkg/ssa"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
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
func (s *StorageManager) Apply(ctx context.Context, instance *apiv1.Instance, createNamespace bool) error {
	instance.LastTransitionTime = time.Now().UTC().Format(time.RFC3339)
	instanceData, err := json.Marshal(instance)
	if err != nil {
		return err
	}

	if createNamespace {
		if err := s.createNamespace(ctx, instance.Namespace); err != nil {
			return err
		}
	}

	secret := s.newSecret(instance.Name, instance.Namespace)
	secret.Data = map[string][]byte{
		storageDataKey: instanceData,
	}

	for labelKey, labelValue := range instance.Labels {
		secret.Labels[labelKey] = labelValue
	}

	opts := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner(ownerRef.Field),
	}
	return s.resManager.Client().Patch(ctx, secret, client.Apply, opts...)
}

// Get retrieves the instance from the storage.
func (s *StorageManager) Get(ctx context.Context, name, namespace string) (*apiv1.Instance, error) {
	secret := s.newSecret(name, namespace)
	secretKey := client.ObjectKeyFromObject(secret)

	err := s.resManager.Client().Get(ctx, secretKey, secret)
	if err != nil {
		return nil, fmt.Errorf("instance storage not found: %w", err)
	}

	if _, ok := secret.Data[storageDataKey]; !ok {
		return nil, fmt.Errorf("instance data not found in Secret/%s", secretKey)
	}

	instance, err := s.decodeInstance(secret.Data[storageDataKey], secret.ObjectMeta)
	if err != nil {
		return nil, fmt.Errorf("invalid instance found in Secret/%s/%s: %w",
			secret.GetNamespace(), secret.GetName(), err)
	}
	return instance, nil
}

// List returns the instances found in the given namespace.
func (s *StorageManager) List(ctx context.Context, namespace, bundle string) ([]*apiv1.Instance, error) {
	var res []*apiv1.Instance
	secretList := &corev1.SecretList{}
	labels := s.getOwnerLabels()
	if bundle != "" {
		labels[apiv1.BundleNameLabelKey] = bundle
	}
	err := s.resManager.Client().List(ctx, secretList, client.InNamespace(namespace), labels)
	if err != nil {
		return res, err
	}

	if len(secretList.Items) == 0 {
		return res, nil
	}

	var secrets = secretList.Items

	// order list by installed date
	sort.Slice(secrets, func(i, j int) bool {
		return secrets[i].CreationTimestamp.Before(&secrets[j].CreationTimestamp)
	})

	for _, secret := range secrets {
		if _, ok := secret.Data[storageDataKey]; !ok {
			return res, fmt.Errorf("instance data not found in Secret/%s/%s",
				secret.GetNamespace(), secret.GetName())
		}

		i, err := s.decodeInstance(secret.Data[storageDataKey], secret.ObjectMeta)
		if err != nil {
			return res, fmt.Errorf("invalid instance found in Secret/%s/%s: %w",
				secret.GetNamespace(), secret.GetName(), err)
		}
		res = append(res, i)
	}

	return res, nil
}

// Delete removes the storage for the given instance name and namespace.
func (s *StorageManager) Delete(ctx context.Context, name, namespace string) error {
	secret := s.newSecret(name, namespace)
	secretKey := client.ObjectKeyFromObject(secret)

	err := s.resManager.Client().Delete(ctx, secret)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete Secret/%s: %w", secretKey, err)
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

func (s *StorageManager) ListNamespaces(ctx context.Context) ([]string, error) {
	nsList := &corev1.NamespaceList{}
	err := s.resManager.Client().List(ctx, nsList)
	if err != nil {
		return nil, err
	}

	res := make([]string, len(nsList.Items))
	for _, ns := range nsList.Items {
		res = append(res, ns.Name)
	}

	return res, nil
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

func (s *StorageManager) decodeInstance(data []byte, objMeta metav1.ObjectMeta) (*apiv1.Instance, error) {
	var instance apiv1.Instance
	err := json.Unmarshal(data, &instance)
	if err != nil {
		return nil, err
	}

	instance.Annotations = objMeta.Annotations
	instance.Labels = objMeta.Labels
	instance.CreationTimestamp = objMeta.CreationTimestamp
	return &instance, nil
}
