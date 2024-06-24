/*
Copyright 2024 Stefan Prodan

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
	"fmt"
	"time"

	"cuelang.org/go/cue"
	"github.com/fluxcd/pkg/ssa"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/runtime"
)

func NewReconciler(log logr.Logger, opts *CommonOptions, timeout time.Duration) *Reconciler {
	reconciler := &Reconciler{
		opts:           opts,
		currentObjects: []*unstructured.Unstructured{},
		staleObjects:   []*unstructured.Unstructured{},
		applyOptions:   runtime.ApplyOptions(opts.Force, timeout),
		waitOptions: ssa.WaitOptions{
			Interval: 5 * time.Second,
			Timeout:  timeout,
			FailFast: true,
		},
		progressStartFn: func(msg string) interface{ Stop() } {
			log.Info(msg)
			return &noopProgressStopper{}
		},
	}
	reconciler.applyOptions.WaitInterval = reconciler.waitOptions.Interval

	return reconciler
}

func (r *Reconciler) Init(ctx context.Context, builder *engine.ModuleBuilder, buildResult cue.Value, instance *engine.BundleInstance, rcg genericclioptions.RESTClientGetter) error {
	finalValues, err := builder.GetDefaultValues()
	if err != nil {
		return fmt.Errorf("failed to extract values: %w", err)
	}

	r.sets, err = builder.GetApplySets(buildResult)
	if err != nil {
		return fmt.Errorf("failed to extract objects: %w", err)
	}

	for _, set := range r.sets {
		r.currentObjects = append(r.currentObjects, set.Objects...)
	}

	r.resourceManager, err = runtime.NewResourceManager(rcg)
	if err != nil {
		return err
	}

	r.resourceManager.SetOwnerLabels(r.currentObjects, instance.Name, instance.Namespace)

	r.storageManager = runtime.NewStorageManager(r.resourceManager)
	storedInstance, err := r.storageManager.Get(ctx, instance.Name, instance.Namespace)
	if err == nil {
		r.instanceExists = true
	}

	isStandaloneInstance := instance.Bundle == ""

	if !r.opts.OverwriteOwnership && r.instanceExists && isStandaloneInstance {
		if currentOwnerBundle := storedInstance.Labels[apiv1.BundleNameLabelKey]; currentOwnerBundle != "" {
			return &InstanceOwnershipConflictErr{{
				InstanceName:       instance.Name,
				CurrentOwnerBundle: currentOwnerBundle,
			}}
		}
	}

	r.instanceManager = runtime.NewInstanceManager(instance.Name, instance.Namespace, finalValues, instance.Module)

	if !isStandaloneInstance {
		if r.instanceManager.Instance.Labels == nil {
			r.instanceManager.Instance.Labels = make(map[string]string)
		}
		r.instanceManager.Instance.Labels[apiv1.BundleNameLabelKey] = instance.Bundle
	}

	if err := r.instanceManager.AddObjects(r.currentObjects); err != nil {
		return fmt.Errorf("adding objects to instance failed: %w", err)
	}

	r.staleObjects, err = r.storageManager.GetStaleObjects(ctx, &r.instanceManager.Instance)
	if err != nil {
		return fmt.Errorf("getting stale objects failed: %w", err)
	}
	return nil
}

func (r *Reconciler) ApplyInstance(ctx context.Context, log logr.Logger, builder *engine.ModuleBuilder, buildResult cue.Value) error {
	if !r.instanceExists {
		if err := r.UpdateStoredInstance(ctx); err != nil {
			return fmt.Errorf("instance init failed: %w", err)
		}
	}

	return kerrors.NewAggregate([]error{
		r.ApplyAllSets(ctx, log, r.Wait),
		r.PostApplyUpdateInventory(ctx, builder, buildResult),
		r.PostApplyPruneStaleObjects(ctx, log, r.WaitForTermination),
	})
}

func (a *Reconciler) Wait(ctx context.Context, log logr.Logger, _ *ssa.ChangeSet, rs *engine.ResourceSet) error {
	doneMsg := ""
	if rs != nil && rs.Name != "" {
		doneMsg = fmt.Sprintf("%s resources ready", rs.Name)
	}
	return a.doWait(ctx, log, rs, "waiting for %d resource(s) to become ready", doneMsg)
}

func (r *Reconciler) doWait(_ context.Context, log logr.Logger, rs *engine.ResourceSet, progressMsgFmt string, doneMsg string) error {
	if !r.opts.Wait {
		return nil
	}
	progress := r.progressStartFn(fmt.Sprintf(progressMsgFmt, len(rs.Objects)))
	err := r.resourceManager.Wait(rs.Objects, r.waitOptions)
	progress.Stop()
	if err != nil {
		return err
	}
	if doneMsg != "" {
		doneMsg = "resources are ready"
	}
	log.Info(doneMsg)
	return nil
}

func (r *Reconciler) WaitForTermination(ctx context.Context, log logr.Logger, cs *ssa.ChangeSet, _ *engine.ResourceSet) error {
	return r.doWaitForTermination(ctx, log, cs, "waiting for %d resource(s) to be finalized")
}

func (r *Reconciler) doWaitForTermination(_ context.Context, _ logr.Logger, cs *ssa.ChangeSet, progressMsgFmt string) error {
	if !r.opts.Wait {
		return nil
	}
	deletedObjects := runtime.SelectObjectsFromSet(cs, ssa.DeletedAction)
	if len(deletedObjects) == 0 {
		return nil
	}
	progress := r.progressStartFn(fmt.Sprintf(progressMsgFmt, len(deletedObjects)))
	err := r.resourceManager.WaitForTermination(deletedObjects, r.waitOptions)
	progress.Stop()
	if err != nil {
		return fmt.Errorf("waiting for termination failed: %w", err)
	}
	return nil
}

func (r *Reconciler) ApplyAllSets(ctx context.Context, log logr.Logger, withChangeSet withChangeSetFunc) error {
	if !r.instanceExists {
		if err := r.UpdateStoredInstance(ctx); err != nil {
			return fmt.Errorf("instance init failed: %w", err)
		}
	}

	multiSet := len(r.sets) > 1
	for s := range r.sets {
		set := r.sets[s]
		if multiSet {
			log.Info(fmt.Sprintf("applying %s", set.Name))
		}

		cs, err := r.ApplyAllStaged(ctx, set)
		if err != nil {
			return err
		}

		if withChangeSet != nil {
			if err := withChangeSet(ctx, log, cs, &set); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Reconciler) ApplyAllStaged(ctx context.Context, set engine.ResourceSet) (*ssa.ChangeSet, error) {
	return r.resourceManager.ApplyAllStaged(ctx, set.Objects, r.applyOptions)
}

func (r *Reconciler) PostApplyPruneStaleObjects(ctx context.Context, log logr.Logger, withChangeSet withChangeSetFunc) error {
	if len(r.staleObjects) == 0 {
		return nil
	}
	deleteOpts := runtime.DeleteOptions(r.Name(), r.Namespace())
	cs, err := r.resourceManager.DeleteAll(ctx, r.staleObjects, deleteOpts)
	if err != nil {
		return fmt.Errorf("pruning objects failed: %w", err)
	}
	if withChangeSet != nil {
		if err := withChangeSet(ctx, log, cs, nil); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) PostApplyUpdateInventory(ctx context.Context, builder *engine.ModuleBuilder, buildResult cue.Value) error {
	r.UpdateImages(builder, buildResult)
	if err := r.UpdateStoredInstance(ctx); err != nil {
		return fmt.Errorf("storing instance failed: %w", err)
	}
	return nil
}

func (r *Reconciler) UpdateStoredInstance(ctx context.Context) error {
	return r.storageManager.Apply(ctx, &r.instanceManager.Instance, true)
}

func (r *Reconciler) UpdateImages(builder *engine.ModuleBuilder, buildResult cue.Value) {
	if images, err := builder.GetContainerImages(buildResult); err == nil {
		r.instanceManager.Instance.Images = images
	}
}

func (r *Reconciler) Name() string { return r.instanceManager.Instance.Name }

func (r *Reconciler) Namespace() string { return r.instanceManager.Instance.Namespace }

func (r *Reconciler) NamespaceExists(ctx context.Context) (bool, error) {
	ok, err := r.storageManager.NamespaceExists(ctx, r.Namespace())
	if err != nil {
		return false, fmt.Errorf("cannot determine if namespace %q already exists: %w", r.Namespace(), err)
	}
	return ok, nil
}
