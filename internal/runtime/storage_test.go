/*
Copyright 2026 Stefan Prodan

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
	"testing"

	"github.com/fluxcd/pkg/ssa"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func newTestStorageManager() *StorageManager {
	man := ssa.NewResourceManager(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), nil, ownerRef)
	return NewStorageManager(man)
}

func TestPendingRevisionLifecycle(t *testing.T) {
	g := NewWithT(t)
	sm := newTestStorageManager()
	ctx := context.Background()

	stored := &apiv1.Instance{}
	stored.Name = "my-instance"
	stored.Namespace = "default"
	stored.Inventory = &apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{{ID: "default_old__ConfigMap", Version: "v1"}}}
	g.Expect(sm.Apply(ctx, stored, false)).ToNot(HaveOccurred())

	pending := &apiv1.Instance{}
	pending.Name = "my-instance"
	pending.Namespace = "default"
	pending.Inventory = &apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{{ID: "default_new__ConfigMap", Version: "v1"}}}

	// The stored record stays untouched while the pending revision is in flight.
	g.Expect(sm.SavePending(ctx, pending)).ToNot(HaveOccurred())
	got, err := sm.GetPending(ctx, "my-instance", "default")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got.Inventory).To(Equal(pending.Inventory))

	kept, err := sm.Get(ctx, "my-instance", "default")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(kept.Inventory).To(Equal(stored.Inventory))

	// Finalizing promotes the pending revision and drops the pending key.
	g.Expect(sm.Apply(ctx, pending, false)).ToNot(HaveOccurred())
	got, err = sm.GetPending(ctx, "my-instance", "default")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got).To(BeNil())

	final, err := sm.Get(ctx, "my-instance", "default")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(final.Inventory).To(Equal(pending.Inventory))
}

func TestPendingRevisionNotSurfacedByList(t *testing.T) {
	g := NewWithT(t)
	sm := newTestStorageManager()
	ctx := context.Background()

	stored := &apiv1.Instance{}
	stored.Name = "my-instance"
	stored.Namespace = "default"
	stored.Inventory = &apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{{ID: "default_old__ConfigMap", Version: "v1"}}}
	g.Expect(sm.Apply(ctx, stored, false)).ToNot(HaveOccurred())

	pending := &apiv1.Instance{}
	pending.Name = "my-instance"
	pending.Namespace = "default"
	pending.Inventory = &apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{{ID: "default_new__ConfigMap", Version: "v1"}}}
	g.Expect(sm.SavePending(ctx, pending)).ToNot(HaveOccurred())

	// List surfaces the applied record, never the in-flight one.
	instances, err := sm.List(ctx, "default", "")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(instances).To(HaveLen(1))
	g.Expect(instances[0].Inventory).To(Equal(stored.Inventory))
}

func TestListAllObjectsCoversPendingInventory(t *testing.T) {
	g := NewWithT(t)
	sm := newTestStorageManager()
	ctx := context.Background()

	ref := func(id string) apiv1.ResourceRef {
		return apiv1.ResourceRef{ID: id, Version: "v1"}
	}

	stored := &apiv1.Instance{}
	stored.Name = "my-instance"
	stored.Namespace = "default"
	stored.Inventory = &apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{
		ref("default_old__ConfigMap"),
		ref("default_shared__ConfigMap"),
	}}
	g.Expect(sm.Apply(ctx, stored, false)).ToNot(HaveOccurred())

	pending := &apiv1.Instance{}
	pending.Name = "my-instance"
	pending.Namespace = "default"
	pending.Inventory = &apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{
		ref("default_shared__ConfigMap"),
		ref("default_new__ConfigMap"),
	}}
	g.Expect(sm.SavePending(ctx, pending)).ToNot(HaveOccurred())

	objects, err := sm.ListAllObjects(ctx, "my-instance", "default")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(objects).To(HaveLen(3))

	names := map[string]bool{}
	for _, obj := range objects {
		names[obj.GetName()] = true
	}
	g.Expect(names).To(HaveKey("old"))
	g.Expect(names).To(HaveKey("shared"))
	g.Expect(names).To(HaveKey("new"))
}

func TestDeleteRemovesPendingRevision(t *testing.T) {
	g := NewWithT(t)
	sm := newTestStorageManager()
	ctx := context.Background()

	stored := &apiv1.Instance{}
	stored.Name = "my-instance"
	stored.Namespace = "default"
	g.Expect(sm.Apply(ctx, stored, false)).ToNot(HaveOccurred())
	g.Expect(sm.SavePending(ctx, stored)).ToNot(HaveOccurred())

	g.Expect(sm.Delete(ctx, "my-instance", "default")).ToNot(HaveOccurred())

	got, err := sm.GetPending(ctx, "my-instance", "default")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got).To(BeNil())

	_, err = sm.Get(ctx, "my-instance", "default")
	g.Expect(err).To(HaveOccurred())
}

func TestListAllObjectsWithNoPending(t *testing.T) {
	g := NewWithT(t)
	sm := newTestStorageManager()
	ctx := context.Background()

	stored := &apiv1.Instance{}
	stored.Name = "my-instance"
	stored.Namespace = "default"
	stored.Inventory = &apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{{ID: "default_only__ConfigMap", Version: "v1"}}}
	g.Expect(sm.Apply(ctx, stored, false)).ToNot(HaveOccurred())

	objects, err := sm.ListAllObjects(ctx, "my-instance", "default")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(objects).To(HaveLen(1))
	g.Expect(objects[0].GetName()).To(Equal("only"))
}

func TestObjectRefRoundtrip(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("apps/v1")
	obj.SetKind("Deployment")
	obj.SetName("web")
	obj.SetNamespace("apps")

	im := InstanceManager{Instance: apiv1.Instance{
		Inventory: &apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{{ID: "apps_web_apps_Deployment", Version: "v1"}}},
	}}
	objects, err := im.ListObjects()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(objects).To(HaveLen(1))
	g.Expect(objects[0].GetKind()).To(Equal("Deployment"))
	g.Expect(objects[0].GetAPIVersion()).To(Equal("apps/v1"))
	g.Expect(objects[0].GetName()).To(Equal("web"))
	g.Expect(objects[0].GetNamespace()).To(Equal("apps"))
}

func TestSavePendingKeepsSecretLabels(t *testing.T) {
	g := NewWithT(t)
	sm := newTestStorageManager()
	ctx := context.Background()

	stored := &apiv1.Instance{}
	stored.Name = "my-instance"
	stored.Namespace = "default"
	stored.Labels = map[string]string{apiv1.BundleNameLabelKey: "my-bundle"}
	stored.Inventory = &apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{{ID: "default_old__ConfigMap", Version: "v1"}}}
	g.Expect(sm.Apply(ctx, stored, false)).ToNot(HaveOccurred())

	pending := &apiv1.Instance{}
	pending.Name = "my-instance"
	pending.Namespace = "default"
	pending.Inventory = &apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{{ID: "default_new__ConfigMap", Version: "v1"}}}
	g.Expect(sm.SavePending(ctx, pending)).ToNot(HaveOccurred())

	// The bundle ownership label must survive the pending write, or bundle
	// listing and delete filtering break after the first upgrade.
	got, err := sm.Get(ctx, "my-instance", "default")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got.Labels).To(HaveKeyWithValue(apiv1.BundleNameLabelKey, "my-bundle"))
}
