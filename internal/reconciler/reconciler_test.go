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

package reconciler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"cuelang.org/go/cue"
	"github.com/fluxcd/pkg/ssa"
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/runtime"
)

var errSentinel = errors.New("sentinel failure")

func ref(id string) apiv1.ResourceRef {
	return apiv1.ResourceRef{ID: id, Version: "v1"}
}

func cm(name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion("v1")
	u.SetKind("ConfigMap")
	u.SetName(name)
	u.SetNamespace("default")
	return u
}

func newTestStorageManager() *runtime.StorageManager {
	man := ssa.NewResourceManager(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), nil, ssa.Owner{
		Field: apiv1.FieldManager,
		Group: fmt.Sprintf("%s.%s", strings.ToLower(apiv1.InstanceKind), apiv1.GroupVersion.Group),
	})
	return runtime.NewStorageManager(man)
}

func newTestReconciler(storage *runtime.StorageManager) *Reconciler {
	r := &Reconciler{
		opts:            &CommonOptions{},
		storageManager:  storage,
		progressStartFn: func(string) interface{ Stop() } { return &noopProgressStopper{} },
	}
	r.instanceManager = runtime.NewInstanceManager("my-instance", "default", "", apiv1.ModuleReference{})
	r.applySetsFn = func(context.Context, logr.Logger) error { return nil }
	r.updateInventoryFn = func(ctx context.Context, _ *engine.ModuleBuilder, _ cue.Value) error {
		return r.UpdateStoredInstance(ctx)
	}
	r.pruneStaleFn = func(context.Context, logr.Logger) error { return nil }
	r.savePendingFn = func(ctx context.Context) error {
		return r.storageManager.SavePending(ctx, &r.instanceManager.Instance)
	}
	return r
}

func (r *Reconciler) storeInventory(inv *apiv1.ResourceInventory) error {
	stored := &apiv1.Instance{}
	stored.Name = r.Name()
	stored.Namespace = r.Namespace()
	stored.Inventory = inv
	if err := r.storageManager.Apply(context.Background(), stored, false); err != nil {
		return err
	}
	r.instanceExists = true
	r.predecessorInventory = inv
	return nil
}

func TestApplyInstallStoresIntendedInventoryUpFront(t *testing.T) {
	g := NewWithT(t)
	storage := newTestStorageManager()
	r := newTestReconciler(storage)
	ctx := context.Background()

	desired := cm("web")
	g.Expect(r.instanceManager.AddObjects([]*unstructured.Unstructured{desired})).ToNot(HaveOccurred())

	pruneCalled := 0
	r.pruneStaleFn = func(context.Context, logr.Logger) error { pruneCalled++; return nil }

	// An apply failure still leaves the intended inventory stored, so a
	// later delete can clean up whatever was created.
	r.applySetsFn = func(context.Context, logr.Logger) error { return errSentinel }
	err := r.ApplyInstance(ctx, logr.Discard(), nil, cue.Value{})
	g.Expect(err).To(MatchError(errSentinel))
	g.Expect(pruneCalled).To(BeZero())

	stored, err := storage.Get(ctx, r.Name(), r.Namespace())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(stored.Inventory).To(Equal(r.instanceManager.Instance.Inventory))

	pending, err := storage.GetPending(ctx, r.Name(), r.Namespace())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(pending).To(BeNil())
}

func TestApplyInstallSuccessFinalizes(t *testing.T) {
	g := NewWithT(t)
	storage := newTestStorageManager()
	r := newTestReconciler(storage)
	ctx := context.Background()

	desired := cm("web")
	g.Expect(r.instanceManager.AddObjects([]*unstructured.Unstructured{desired})).ToNot(HaveOccurred())

	err := r.ApplyInstance(ctx, logr.Discard(), nil, cue.Value{})
	g.Expect(err).ToNot(HaveOccurred())

	stored, err := storage.Get(ctx, r.Name(), r.Namespace())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(stored.Inventory).To(Equal(r.instanceManager.Instance.Inventory))
}

func TestApplyUpgradeRecordsPendingBeforeApply(t *testing.T) {
	g := NewWithT(t)
	storage := newTestStorageManager()
	r := newTestReconciler(storage)
	ctx := context.Background()

	g.Expect(r.storeInventory(&apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{ref("default_old__ConfigMap")}})).ToNot(HaveOccurred())
	g.Expect(r.instanceManager.AddObjects([]*unstructured.Unstructured{cm("web")})).ToNot(HaveOccurred())

	// The pending record is durable before the first apply call.
	pendingSeenAtApply := false
	r.applySetsFn = func(ctx context.Context, _ logr.Logger) error {
		pending, err := storage.GetPending(ctx, r.Name(), r.Namespace())
		if err != nil {
			return err
		}
		pendingSeenAtApply = pending != nil
		return nil
	}

	err := r.ApplyInstance(ctx, logr.Discard(), nil, cue.Value{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(pendingSeenAtApply).To(BeTrue())

	// Finalize promotes the pending record and clears it.
	pending, err := storage.GetPending(ctx, r.Name(), r.Namespace())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(pending).To(BeNil())

	stored, err := storage.Get(ctx, r.Name(), r.Namespace())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(stored.Inventory).To(Equal(r.instanceManager.Instance.Inventory))
}

func TestApplyUpgradeSkipsPendingWhenInventoryUnchanged(t *testing.T) {
	g := NewWithT(t)
	storage := newTestStorageManager()
	r := newTestReconciler(storage)
	ctx := context.Background()

	inv := &apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{ref("default_web__ConfigMap")}}
	g.Expect(r.storeInventory(inv)).ToNot(HaveOccurred())
	g.Expect(r.instanceManager.AddObjects([]*unstructured.Unstructured{cm("web")})).ToNot(HaveOccurred())

	err := r.ApplyInstance(ctx, logr.Discard(), nil, cue.Value{})
	g.Expect(err).ToNot(HaveOccurred())

	pending, err := storage.GetPending(ctx, r.Name(), r.Namespace())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(pending).To(BeNil())
}

func TestApplyUpgradeApplyFailurePrunesNothing(t *testing.T) {
	g := NewWithT(t)
	storage := newTestStorageManager()
	r := newTestReconciler(storage)
	ctx := context.Background()

	g.Expect(r.storeInventory(&apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{ref("default_old__ConfigMap")}})).ToNot(HaveOccurred())
	g.Expect(r.instanceManager.AddObjects([]*unstructured.Unstructured{cm("web")})).ToNot(HaveOccurred())

	pruneCalled, finalizeCalled := 0, 0
	r.applySetsFn = func(context.Context, logr.Logger) error { return errSentinel }
	r.pruneStaleFn = func(context.Context, logr.Logger) error { pruneCalled++; return nil }
	r.updateInventoryFn = func(context.Context, *engine.ModuleBuilder, cue.Value) error { finalizeCalled++; return nil }

	err := r.ApplyInstance(ctx, logr.Discard(), nil, cue.Value{})
	g.Expect(err).To(MatchError(errSentinel))
	g.Expect(pruneCalled).To(BeZero())
	g.Expect(finalizeCalled).To(BeZero())

	// The pending record stays and the stored inventory still describes the
	// predecessor revision.
	pending, err := storage.GetPending(ctx, r.Name(), r.Namespace())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(pending).ToNot(BeNil())

	stored, err := storage.Get(ctx, r.Name(), r.Namespace())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(stored.Inventory.Entries).To(Equal([]apiv1.ResourceRef{ref("default_old__ConfigMap")}))
}

func TestApplyUpgradeReadinessFailureStillPrunes(t *testing.T) {
	g := NewWithT(t)
	storage := newTestStorageManager()
	r := newTestReconciler(storage)
	ctx := context.Background()

	g.Expect(r.storeInventory(&apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{ref("default_old__ConfigMap")}})).ToNot(HaveOccurred())
	g.Expect(r.instanceManager.AddObjects([]*unstructured.Unstructured{cm("web")})).ToNot(HaveOccurred())

	pruneCalled, finalizeCalled := 0, 0
	waitErr := errors.New("not ready")
	r.applySetsFn = func(context.Context, logr.Logger) error { return &ReadinessError{Err: waitErr} }
	r.pruneStaleFn = func(context.Context, logr.Logger) error { pruneCalled++; return nil }
	r.updateInventoryFn = func(context.Context, *engine.ModuleBuilder, cue.Value) error { finalizeCalled++; return nil }

	err := r.ApplyInstance(ctx, logr.Discard(), nil, cue.Value{})
	g.Expect(err).To(MatchError(waitErr))
	g.Expect(pruneCalled).To(Equal(1))
	g.Expect(finalizeCalled).To(BeZero())

	// The pending record is kept so the retry can finalize once ready.
	pending, err := storage.GetPending(ctx, r.Name(), r.Namespace())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(pending).ToNot(BeNil())
}

func TestApplyUpgradePruneFailureKeepsPending(t *testing.T) {
	g := NewWithT(t)
	storage := newTestStorageManager()
	r := newTestReconciler(storage)
	ctx := context.Background()

	g.Expect(r.storeInventory(&apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{ref("default_old__ConfigMap")}})).ToNot(HaveOccurred())
	g.Expect(r.instanceManager.AddObjects([]*unstructured.Unstructured{cm("web")})).ToNot(HaveOccurred())

	finalizeCalled := 0
	r.pruneStaleFn = func(context.Context, logr.Logger) error { return errSentinel }
	r.updateInventoryFn = func(context.Context, *engine.ModuleBuilder, cue.Value) error { finalizeCalled++; return nil }

	err := r.ApplyInstance(ctx, logr.Discard(), nil, cue.Value{})
	g.Expect(err).To(MatchError(errSentinel))
	g.Expect(finalizeCalled).To(BeZero())

	pending, err := storage.GetPending(ctx, r.Name(), r.Namespace())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(pending).ToNot(BeNil())
}

func TestStaleObjectsCoverPredecessorAndPending(t *testing.T) {
	g := NewWithT(t)
	storage := newTestStorageManager()
	r := newTestReconciler(storage)
	ctx := context.Background()

	// Stored revision owns old and shared; the unfinished pending revision
	// owns shared and newer. The desired render owns only shared.
	g.Expect(r.storeInventory(&apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{
		ref("default_old__ConfigMap"),
		ref("default_shared__ConfigMap"),
	}})).ToNot(HaveOccurred())
	pending := &apiv1.Instance{}
	pending.Name = r.Name()
	pending.Namespace = r.Namespace()
	pending.Inventory = &apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{
		ref("default_shared__ConfigMap"),
		ref("default_newer__ConfigMap"),
	}}
	g.Expect(storage.SavePending(ctx, pending)).ToNot(HaveOccurred())

	shared := cm("shared")
	g.Expect(r.instanceManager.AddObjects([]*unstructured.Unstructured{shared})).ToNot(HaveOccurred())

	// The stale computation covers both the stored and the pending records.
	g.Expect(r.computeStaleObjects(ctx, r.Name(), r.Namespace())).ToNot(HaveOccurred())
	stale := r.staleObjects

	names := map[string]bool{}
	for _, obj := range stale {
		names[obj.GetName()] = true
	}
	g.Expect(names).To(Equal(map[string]bool{"old": true, "newer": true}))
}

func TestInteractiveApplyInstallStoresIntendedInventoryFirst(t *testing.T) {
	g := NewWithT(t)
	storage := newTestStorageManager()
	r := NewInteractiveReconciler(logr.Discard(), &CommonOptions{}, &InteractiveOptions{}, time.Second)
	r.storageManager = storage
	r.instanceManager = runtime.NewInstanceManager("my-instance", "default", "", apiv1.ModuleReference{})
	ctx := context.Background()

	g.Expect(r.instanceManager.AddObjects([]*unstructured.Unstructured{cm("web")})).ToNot(HaveOccurred())

	pruneCalled, finalizeCalled := 0, 0
	r.applySetsFn = func(context.Context, logr.Logger) error { return errSentinel }
	r.pruneStaleFn = func(context.Context, logr.Logger) error { pruneCalled++; return nil }
	r.updateInventoryFn = func(context.Context, *engine.ModuleBuilder, cue.Value) error { finalizeCalled++; return nil }

	err := r.ApplyInstance(ctx, logr.Discard(), nil, cue.Value{})
	g.Expect(err).To(MatchError(errSentinel))
	g.Expect(pruneCalled).To(BeZero())
	g.Expect(finalizeCalled).To(BeZero())

	stored, err := storage.Get(ctx, r.Name(), r.Namespace())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(stored.Inventory).To(Equal(r.instanceManager.Instance.Inventory))
}

func TestInteractiveApplyUpgradeRecordsPending(t *testing.T) {
	g := NewWithT(t)
	storage := newTestStorageManager()
	r := NewInteractiveReconciler(logr.Discard(), &CommonOptions{}, &InteractiveOptions{}, time.Second)
	r.storageManager = storage
	r.instanceManager = runtime.NewInstanceManager("my-instance", "default", "", apiv1.ModuleReference{})
	ctx := context.Background()

	// Seed the stored predecessor revision.
	stored := &apiv1.Instance{}
	stored.Name = r.Name()
	stored.Namespace = r.Namespace()
	stored.Inventory = &apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{ref("default_old__ConfigMap")}}
	g.Expect(storage.Apply(ctx, stored, false)).ToNot(HaveOccurred())
	r.instanceExists = true
	r.predecessorInventory = stored.Inventory

	g.Expect(r.instanceManager.AddObjects([]*unstructured.Unstructured{cm("web")})).ToNot(HaveOccurred())

	// The pending record is durable before the first apply call.
	pendingSeenAtApply := false
	r.applySetsFn = func(ctx context.Context, _ logr.Logger) error {
		pending, err := storage.GetPending(ctx, r.Name(), r.Namespace())
		if err != nil {
			return err
		}
		pendingSeenAtApply = pending != nil
		return nil
	}
	r.updateInventoryFn = func(ctx context.Context, _ *engine.ModuleBuilder, _ cue.Value) error {
		return r.UpdateStoredInstance(ctx)
	}

	err := r.ApplyInstance(ctx, logr.Discard(), nil, cue.Value{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(pendingSeenAtApply).To(BeTrue())

	// Finalize promotes the pending record and clears it.
	pending, err := storage.GetPending(ctx, r.Name(), r.Namespace())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(pending).To(BeNil())

	stored2, err := storage.Get(ctx, r.Name(), r.Namespace())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(stored2.Inventory).To(Equal(r.instanceManager.Instance.Inventory))
}

func TestApplyUpgradeReadinessAndPruneFailureReportsBoth(t *testing.T) {
	g := NewWithT(t)
	storage := newTestStorageManager()
	r := newTestReconciler(storage)
	ctx := context.Background()

	g.Expect(r.storeInventory(&apiv1.ResourceInventory{Entries: []apiv1.ResourceRef{ref("default_old__ConfigMap")}})).ToNot(HaveOccurred())
	g.Expect(r.instanceManager.AddObjects([]*unstructured.Unstructured{cm("web")})).ToNot(HaveOccurred())

	waitErr := errors.New("not ready")
	pruneErr := errors.New("prune failed")
	r.applySetsFn = func(context.Context, logr.Logger) error { return &ReadinessError{Err: waitErr} }
	r.pruneStaleFn = func(context.Context, logr.Logger) error { return pruneErr }
	r.updateInventoryFn = func(context.Context, *engine.ModuleBuilder, cue.Value) error { return nil }

	err := r.ApplyInstance(ctx, logr.Discard(), nil, cue.Value{})
	g.Expect(errors.Is(err, waitErr)).To(BeTrue())
	g.Expect(errors.Is(err, pruneErr)).To(BeTrue())

	// The pending record stays so the retry can finalize once ready.
	pending, err := storage.GetPending(ctx, r.Name(), r.Namespace())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(pending).ToNot(BeNil())
}
