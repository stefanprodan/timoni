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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fluxcd/cli-utils/pkg/kstatus/polling"
	"github.com/fluxcd/cli-utils/pkg/kstatus/polling/clusterreader"
	pollingEngine "github.com/fluxcd/cli-utils/pkg/kstatus/polling/engine"
	"github.com/fluxcd/pkg/ssa"
	ssautil "github.com/fluxcd/pkg/ssa/utils"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// ownerRef contains the server-side apply field manager and ownership labels group.
var ownerRef = ssa.Owner{
	Field: apiv1.FieldManager,
	Group: fmt.Sprintf("%s.%s", strings.ToLower(apiv1.InstanceKind), apiv1.GroupVersion.Group),
}

// NewResourceManager creates a ResourceManager for the given cluster.
// The manager's client and status poller share a dynamic RESTMapper that
// discovers kinds registered by CRDs applied during the current run.
// The optional factories add custom kstatus readers to the status poller,
// next to the built-in Kubernetes Job one.
func NewResourceManager(rcg genericclioptions.RESTClientGetter, readers ...StatusReaderFactory) (*ssa.ResourceManager, error) {
	cfg, err := rcg.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig failed: %w", err)
	}

	// bump limits
	cfg.QPS = 100.0
	cfg.Burst = 300

	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, err
	}

	// The dynamic RESTMapper reloads the API discovery data on unknown kinds.
	restMapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)
	if err != nil {
		return nil, err
	}

	kubeClient, err := client.New(cfg, client.Options{
		HTTPClient: httpClient,
		Mapper:     restMapper,
		Scheme:     defaultScheme(),
	})
	if err != nil {
		return nil, err
	}

	// The custom readers are registered before the built-in Job one
	// so that module-defined health checks take precedence.
	var statusReaders []pollingEngine.StatusReader
	for _, factory := range readers {
		statusReaders = append(statusReaders, factory(restMapper))
	}
	statusReaders = append(statusReaders, NewCustomJobStatusReader(restMapper))

	kubePoller := polling.NewStatusPoller(kubeClient, restMapper, polling.Options{
		CustomStatusReaders:  statusReaders,
		ClusterReaderFactory: pollingEngine.ClusterReaderFactoryFunc(clusterreader.NewDirectClusterReader),
	})

	man := ssa.NewResourceManager(kubeClient, kubePoller, ownerRef)

	// bump the server-side apply concurrency
	man.SetConcurrency(4)

	return man, nil
}

// SelectObjectsFromSet returns a list of Kubernetes objects from the given changeset filtered by action.
func SelectObjectsFromSet(set *ssa.ChangeSet, action ssa.Action) []*unstructured.Unstructured {
	var objects []*unstructured.Unstructured
	for _, entry := range set.Entries {
		if entry.Action == action {
			u := &unstructured.Unstructured{}
			u.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   entry.ObjMetadata.GroupKind.Group,
				Kind:    entry.ObjMetadata.GroupKind.Kind,
				Version: entry.GroupVersion,
			})
			u.SetName(entry.ObjMetadata.Name)
			u.SetNamespace(entry.ObjMetadata.Namespace)
			objects = append(objects, u)
		}
	}
	return objects
}

// ApplyOptions returns the default options for server-side apply operations.
// The cleanup options transfer the ownership of objects managed with kubectl
// or Helm to Timoni, and remove their tracking metadata.
func ApplyOptions(force bool, wait time.Duration) ssa.ApplyOptions {
	return ssa.ApplyOptions{
		Force: force,
		ForceSelector: map[string]string{
			apiv1.ForceAction: apiv1.EnabledValue,
		},
		IfNotPresentSelector: map[string]string{
			apiv1.IfNotPresentAction: apiv1.EnabledValue,
		},
		Cleanup: ssa.ApplyCleanupOptions{
			// Remove the kubectl and Helm tracking metadata.
			Annotations: []string{
				corev1.LastAppliedConfigAnnotation,
				"meta.helm.sh/release-name",
				"meta.helm.sh/release-namespace",
			},
			// Take ownership of existing objects and
			// undo changes made with kubectl or Helm.
			FieldManagers: takeOwnershipFrom(),
		},
		// Apply the RBAC Roles in a separate stage before the other
		// namespaced objects, so that the RoleBindings referencing them
		// pass the RBAC escalation checks when applied by a user which
		// is not a cluster-admin.
		CustomStageKinds: map[schema.GroupKind]struct{}{
			{Group: "rbac.authorization.k8s.io", Kind: "Role"}: {},
		},
		WaitTimeout: wait,
	}
}

// TakeOwnership transfers the ownership of the given objects' fields from
// kubectl and Helm to Timoni's field manager before server-side applying
// them, so that the apply replaces the fields set with these tools instead
// of merging with them. Objects not found on the cluster, without matching
// field managers, or annotated with the if-not-present action are skipped.
func TakeOwnership(ctx context.Context, kubeClient client.Client, objects []*unstructured.Unstructured) error {
	skipSelector := map[string]string{
		apiv1.IfNotPresentAction: apiv1.EnabledValue,
	}

	for _, object := range objects {
		if ssautil.AnyInMetadata(object, skipSelector) {
			continue
		}

		existing := &unstructured.Unstructured{}
		existing.SetGroupVersionKind(object.GroupVersionKind())
		if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(object), existing); err != nil {
			if apierrors.IsNotFound(err) || apimeta.IsNoMatchError(err) {
				continue
			}
			return fmt.Errorf("%s failed to read object: %w", ssautil.FmtUnstructured(object), err)
		}

		if ssautil.AnyInMetadata(existing, skipSelector) {
			continue
		}

		patch, err := ssa.PatchReplaceFieldsManagers(existing, takeOwnershipFrom(), ownerRef.Field)
		if err != nil {
			return fmt.Errorf("%s failed to compute ownership patch: %w", ssautil.FmtUnstructured(existing), err)
		}
		if len(patch) == 0 {
			continue
		}

		rawPatch, err := json.Marshal(patch)
		if err != nil {
			return err
		}
		if err := kubeClient.Patch(ctx, existing, client.RawPatch(types.JSONPatchType, rawPatch),
			client.FieldOwner(ownerRef.Field)); err != nil {
			return fmt.Errorf("%s failed to take ownership: %w", ssautil.FmtUnstructured(existing), err)
		}
	}

	return nil
}

// takeOwnershipFrom returns the list of field managers whose ownership
// over objects is transferred to Timoni during server-side apply.
func takeOwnershipFrom() []ssa.FieldManager {
	return []ssa.FieldManager{
		{
			// to take over objects managed with Helm v3 client-side apply
			Name:          "helm",
			OperationType: metav1.ManagedFieldsOperationUpdate,
			ExactMatch:    true,
		},
		{
			// to take over objects managed with Helm v4 server-side apply
			Name:          "helm",
			OperationType: metav1.ManagedFieldsOperationApply,
			ExactMatch:    true,
		},
		{
			// to take over changes made with 'kubectl apply'
			Name:          "kubectl",
			OperationType: metav1.ManagedFieldsOperationUpdate,
		},
		{
			// to take over changes made with 'kubectl apply --server-side'
			Name:          "before-first-apply",
			OperationType: metav1.ManagedFieldsOperationUpdate,
		},
		{
			// to take over changes made with 'kubectl apply --server-side --force-conflicts'
			Name:          "kubectl",
			OperationType: metav1.ManagedFieldsOperationApply,
		},
	}
}

// DeleteOptions returns the default options for delete operations.
func DeleteOptions(name, namespace string) ssa.DeleteOptions {
	return ssa.DeleteOptions{
		PropagationPolicy: metav1.DeletePropagationBackground,
		Inclusions: map[string]string{
			ownerRef.Group + "/name":      name,
			ownerRef.Group + "/namespace": namespace,
		},
		Exclusions: map[string]string{
			apiv1.PruneAction: apiv1.DisabledValue,
		},
	}
}

func defaultScheme() *apiruntime.Scheme {
	scheme := apiruntime.NewScheme()
	_ = apiextensionsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	return scheme
}

// ToUnstructured converts a runtime.Object into an Unstructured object.
func ToUnstructured(obj apiruntime.Object) (*unstructured.Unstructured, error) {
	// If the incoming object is already unstructured, perform a deep copy first
	// otherwise DefaultUnstructuredConverter ends up returning the inner map without
	// making a copy.
	if _, ok := obj.(apiruntime.Unstructured); ok {
		obj = obj.DeepCopyObject()
	}
	rawMap, err := apiruntime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: rawMap}, nil
}
