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

package apply

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"github.com/fluxcd/pkg/ssa"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/dyff"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/logger"
	"github.com/stefanprodan/timoni/internal/runtime"
)

type CommonOptions struct {
	Dir                string
	Wait               bool
	Force              bool
	OverwriteOwnership bool
}

type InteractiveOptions struct {
	DryRun     bool
	Diff       bool
	DiffOutput io.Writer

	ProgressStart ProgressStarter
}

type InstanceApplier struct {
	opts *CommonOptions

	instanceExists bool

	sets []engine.ResourceSet

	currentObjects, staleObjects []*unstructured.Unstructured

	storageManager  *runtime.StorageManager
	instanceManager *runtime.InstanceManager
	resourceManager *ssa.ResourceManager

	applyOptions ssa.ApplyOptions
	waitOptions  ssa.WaitOptions

	progressStart ProgressStarter
}

type (
	ProgressStarter func(string) ProgressStopper
	ProgressStopper interface{ Stop() }
)

type noopProgressStopper struct{}

func (n *noopProgressStopper) Stop() {}

type withChangeSetFunc func(context.Context, logr.Logger, *ssa.ChangeSet, *engine.ResourceSet) error

func NewInstanceApplier(log logr.Logger, opts *CommonOptions, timeout time.Duration) *InstanceApplier {
	applier := &InstanceApplier{
		opts:           opts,
		currentObjects: []*unstructured.Unstructured{},
		staleObjects:   []*unstructured.Unstructured{},
		applyOptions:   runtime.ApplyOptions(opts.Force, timeout),
		waitOptions: ssa.WaitOptions{
			Interval: 5 * time.Second,
			Timeout:  timeout,
			FailFast: true,
		},
		progressStart: func(msg string) ProgressStopper {
			log.Info(msg)
			return &noopProgressStopper{}
		},
	}
	applier.applyOptions.WaitInterval = applier.waitOptions.Interval

	return applier
}

func (a *InstanceApplier) Init(ctx context.Context, builder *engine.ModuleBuilder, buildResult cue.Value, instance *engine.BundleInstance, rcg genericclioptions.RESTClientGetter) error {
	finalValues, err := builder.GetDefaultValues()
	if err != nil {
		return fmt.Errorf("failed to extract values: %w", err)
	}

	a.sets, err = builder.GetApplySets(buildResult)
	if err != nil {
		return fmt.Errorf("failed to extract objects: %w", err)
	}

	for _, set := range a.sets {
		a.currentObjects = append(a.currentObjects, set.Objects...)
	}

	a.resourceManager, err = runtime.NewResourceManager(rcg)
	if err != nil {
		return err
	}

	a.resourceManager.SetOwnerLabels(a.currentObjects, instance.Name, instance.Namespace)

	a.storageManager = runtime.NewStorageManager(a.resourceManager)
	storedInstance, err := a.storageManager.Get(ctx, instance.Name, instance.Namespace)
	if err == nil {
		a.instanceExists = true
	}

	isStandaloneInstance := instance.Bundle == ""

	if !a.opts.OverwriteOwnership && a.instanceExists && isStandaloneInstance {
		if currentOwnerBundle := storedInstance.Labels[apiv1.BundleNameLabelKey]; currentOwnerBundle != "" {
			return &InstanceOwnershipConflictErr{{
				InstanceName:       instance.Name,
				CurrentOwnerBundle: currentOwnerBundle,
			}}
		}
	}

	a.instanceManager = runtime.NewInstanceManager(instance.Name, instance.Namespace, finalValues, instance.Module)

	if !isStandaloneInstance {
		if a.instanceManager.Instance.Labels == nil {
			a.instanceManager.Instance.Labels = make(map[string]string)
		}
		a.instanceManager.Instance.Labels[apiv1.BundleNameLabelKey] = instance.Bundle
	}

	if err := a.instanceManager.AddObjects(a.currentObjects); err != nil {
		return fmt.Errorf("adding objects to instance failed: %w", err)
	}

	a.staleObjects, err = a.storageManager.GetStaleObjects(ctx, &a.instanceManager.Instance)
	if err != nil {
		return fmt.Errorf("getting stale objects failed: %w", err)
	}
	return nil
}

func (a *InstanceApplier) ApplyInstance(ctx context.Context, log logr.Logger, builder *engine.ModuleBuilder, buildResult cue.Value, opts InteractiveOptions) error {
	if !a.instanceExists {
		if err := a.UpdateStoredInstance(ctx); err != nil {
			return fmt.Errorf("instance init failed: %w", err)
		}
	}

	return kerrors.NewAggregate([]error{
		a.ApplyAllSets(ctx, log, a.Wait),
		a.PostApplyInventory(ctx, builder, buildResult),
		a.PostApplyPruneStaleObjects(ctx, log, a.WaitForTermination),
	})
}

func (a *InstanceApplier) ApplyInstanceInteractively(ctx context.Context, log logr.Logger, builder *engine.ModuleBuilder, buildResult cue.Value, opts InteractiveOptions) error {
	if opts.DiffOutput == nil {
		opts.DiffOutput = io.Discard
	}

	if opts.ProgressStart != nil {
		a.progressStart = opts.ProgressStart
	}

	namespaceExists, err := a.NamespaceExists(ctx)
	if err != nil {
		return err
	}

	if opts.DryRun || opts.Diff {
		if !namespaceExists {
			log.Info(logger.ColorizeJoin(logger.ColorizeSubject("Namespace/"+a.Namespace()),
				ssa.CreatedAction, logger.DryRunServer))
		}
		if err := a.DryRunDiff(logr.NewContext(ctx, log), namespaceExists, opts); err != nil {
			return err
		}

		log.Info(logger.ColorizeJoin("applied successfully", logger.ColorizeDryRun("(server dry run)")))
		return nil
	}

	if !a.instanceExists {
		log.Info(fmt.Sprintf("installing %s in namespace %s",
			logger.ColorizeSubject(a.Name()), logger.ColorizeSubject(a.Namespace())))

		if err := a.UpdateStoredInstance(ctx); err != nil {
			return fmt.Errorf("instance init failed: %w", err)
		}

		if !namespaceExists {
			log.Info(logger.ColorizeJoin(logger.ColorizeSubject("Namespace/"+a.Namespace()), ssa.CreatedAction))
		}
	} else {
		log.Info(fmt.Sprintf("upgrading %s in namespace %s",
			logger.ColorizeSubject(a.Name()), logger.ColorizeSubject(a.Namespace())))
	}

	return kerrors.NewAggregate([]error{
		a.ApplyAllSets(ctx, log, a.WaitInteractive),
		a.PostApplyInventory(ctx, builder, buildResult),
		a.PostApplyPruneStaleObjects(ctx, log, a.WaitForTerminationInteractive),
	})
}

func (a *InstanceApplier) Wait(ctx context.Context, log logr.Logger, _ *ssa.ChangeSet, rs *engine.ResourceSet) error {
	doneMsg := ""
	if rs != nil && rs.Name != "" {
		doneMsg = fmt.Sprintf("%s resources ready", rs.Name)
	}
	return a.doWait(ctx, log, rs, "waiting for %d resource(s) to become ready", doneMsg)
}

func (a *InstanceApplier) WaitInteractive(ctx context.Context, log logr.Logger, cs *ssa.ChangeSet, rs *engine.ResourceSet) error {
	for _, change := range cs.Entries {
		log.Info(logger.ColorizeJoin(change))
	}
	doneMsg := ""
	if rs != nil && rs.Name != "" {
		doneMsg = fmt.Sprintf("%s resources %s", rs.Name, logger.ColorizeReady("ready"))
	}
	return a.doWait(ctx, log, rs, "waiting for %d resource(s) to become ready...", doneMsg)
}

func (a *InstanceApplier) doWait(_ context.Context, log logr.Logger, rs *engine.ResourceSet, progressMsgFmt string, doneMsg string) error {
	if !a.opts.Wait {
		return nil
	}
	progress := a.progressStart(fmt.Sprintf(progressMsgFmt, len(rs.Objects)))
	err := a.resourceManager.Wait(rs.Objects, a.waitOptions)
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

func (a *InstanceApplier) WaitForTermination(ctx context.Context, log logr.Logger, cs *ssa.ChangeSet, _ *engine.ResourceSet) error {
	return a.doWaitForTermination(ctx, log, cs, "waiting for %d resource(s) to be finalized")
}

func (a *InstanceApplier) WaitForTerminationInteractive(ctx context.Context, log logr.Logger, cs *ssa.ChangeSet, _ *engine.ResourceSet) error {
	for _, change := range cs.Entries {
		log.Info(logger.ColorizeJoin(change))
	}
	return a.doWaitForTermination(ctx, log, cs, "waiting for %d resource(s) to be finalized...")
}

func (a *InstanceApplier) doWaitForTermination(_ context.Context, log logr.Logger, cs *ssa.ChangeSet, progressMsgFmt string) error {
	if !a.opts.Wait {
		return nil
	}
	deletedObjects := runtime.SelectObjectsFromSet(cs, ssa.DeletedAction)
	if len(deletedObjects) == 0 {
		return nil
	}
	progress := a.progressStart(fmt.Sprintf(progressMsgFmt, len(deletedObjects)))
	err := a.resourceManager.WaitForTermination(deletedObjects, a.waitOptions)
	progress.Stop()
	if err != nil {
		return fmt.Errorf("waiting for termination failed: %w", err)
	}
	return nil
}

func (a *InstanceApplier) ApplyAllSets(ctx context.Context, log logr.Logger, withChangeSet withChangeSetFunc) error {
	if !a.instanceExists {
		if err := a.UpdateStoredInstance(ctx); err != nil {
			return fmt.Errorf("instance init failed: %w", err)
		}
	}

	multiSet := len(a.sets) > 1
	for s := range a.sets {
		set := a.sets[s]
		if multiSet {
			log.Info(fmt.Sprintf("applying %s", set.Name))
		}

		cs, err := a.ApplyAllStaged(ctx, set)
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

func (a *InstanceApplier) PostApplyPruneStaleObjects(ctx context.Context, log logr.Logger, withChangeSet withChangeSetFunc) error {
	if len(a.staleObjects) == 0 {
		return nil
	}
	deleteOpts := runtime.DeleteOptions(a.Name(), a.Namespace())
	cs, err := a.resourceManager.DeleteAll(ctx, a.staleObjects, deleteOpts)
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

func (a *InstanceApplier) PostApplyInventory(ctx context.Context, builder *engine.ModuleBuilder, buildResult cue.Value) error {
	a.UpdateImages(builder, buildResult)
	if err := a.UpdateStoredInstance(ctx); err != nil {
		return fmt.Errorf("storing instance failed: %w", err)
	}
	return nil
}

func (a *InstanceApplier) UpdateStoredInstance(ctx context.Context) error {
	return a.storageManager.Apply(ctx, &a.instanceManager.Instance, true)
}

func (a *InstanceApplier) UpdateImages(builder *engine.ModuleBuilder, buildResult cue.Value) {
	if images, err := builder.GetContainerImages(buildResult); err == nil {
		a.instanceManager.Instance.Images = images
	}
}

func (a *InstanceApplier) Name() string { return a.instanceManager.Instance.Name }

func (a *InstanceApplier) Namespace() string { return a.instanceManager.Instance.Namespace }

func (a *InstanceApplier) NamespaceExists(ctx context.Context) (bool, error) {
	ok, err := a.storageManager.NamespaceExists(ctx, a.Namespace())
	if err != nil {
		return false, fmt.Errorf("cannot determine if namespace %q already exists: %w", a.Namespace(), err)
	}
	return ok, nil
}

type InstanceOwnershipConflict struct{ InstanceName, CurrentOwnerBundle string }
type InstanceOwnershipConflictErr []InstanceOwnershipConflict

func (e *InstanceOwnershipConflictErr) Error() string {
	s := &strings.Builder{}
	s.WriteString("instance ownership conflict encountered. ")
	s.WriteString("Conflict: ")
	numConflicts := len(*e)
	for i, c := range *e {
		if c.CurrentOwnerBundle != "" {
			s.WriteString(fmt.Sprintf("instance %q exists and is managed by bundle %q", c.InstanceName, c.CurrentOwnerBundle))
		} else {
			s.WriteString(fmt.Sprintf("instance %q exists and is managed by no bundle", c.InstanceName))
		}
		if numConflicts > 1 && i != numConflicts {
			s.WriteString("; ")
		}
	}
	return s.String()
}

func (a *InstanceApplier) ApplyAllStaged(ctx context.Context, set engine.ResourceSet) (*ssa.ChangeSet, error) {
	return a.resourceManager.ApplyAllStaged(ctx, set.Objects, a.applyOptions)
}

func (a *InstanceApplier) DryRunDiff(ctx context.Context, namespaceExists bool, opts InteractiveOptions) error {
	return dyff.InstanceDryRunDiff(
		ctx,
		a.resourceManager,
		a.currentObjects,
		a.staleObjects,
		namespaceExists,
		a.opts.Dir,
		opts.Diff,
		opts.DiffOutput,
	)
}
