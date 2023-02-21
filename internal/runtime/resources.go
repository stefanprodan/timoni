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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"strings"
	"time"

	"github.com/fluxcd/pkg/ssa"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// ownerRef contains the server-side apply field manager and ownership labels group.
var ownerRef = ssa.Owner{
	Field: apiv1.FieldManager,
	Group: fmt.Sprintf("%s.%s", strings.ToLower(apiv1.InstanceKind), apiv1.GroupVersion.Group),
}

// NewResourceManager creates a ResourceManager for the given cluster.
func NewResourceManager(rcg genericclioptions.RESTClientGetter) (*ssa.ResourceManager, error) {
	cfg, err := rcg.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig failed: %w", err)
	}

	// bump limits
	cfg.QPS = 100.0
	cfg.Burst = 300

	restMapper, err := rcg.ToRESTMapper()
	if err != nil {
		return nil, err
	}

	kubeClient, err := client.New(cfg, client.Options{Mapper: restMapper, Scheme: defaultScheme()})
	if err != nil {
		return nil, err
	}

	kubePoller := polling.NewStatusPoller(kubeClient, restMapper, polling.Options{})

	return ssa.NewResourceManager(kubeClient, kubePoller, ownerRef), nil
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
func ApplyOptions(force bool, wait time.Duration) ssa.ApplyOptions {
	return ssa.ApplyOptions{
		Force: force,
		ForceSelector: map[string]string{
			apiv1.ForceAction: apiv1.EnabledValue,
		},
		WaitTimeout: wait,
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
