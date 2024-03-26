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
	"errors"
	"fmt"
	"io"
	"time"

	"cuelang.org/go/cue"
	"github.com/fluxcd/pkg/ssa"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/dyff"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/logger"
	"github.com/stefanprodan/timoni/internal/runtime"
)

type Options struct {
	Dir                string
	DryRun             bool
	Diff               bool
	Wait               bool
	Force              bool
	OverwriteOwnership bool
	DiffOutput         io.Writer

	KubeConfigFlags *genericclioptions.ConfigFlags

	ProgressStart         func(string) ProgressStopper
	OwnershipConflictHint string
}

type ProgressStopper interface{ Stop() }

type noopProgressStopper struct{}

func (n *noopProgressStopper) Stop() {}

type InstanceApplier struct {
	opts Options

	instanceExists, namespaceExists, isStandaloneInstance bool

	sets []engine.ResourceSet

	currentObjects, staleObjects, deletedObjects []*unstructured.Unstructured

	storageManager  *runtime.StorageManager
	instanceManager *runtime.InstanceManager
	resourceManager *ssa.ResourceManager

	applyOptions ssa.ApplyOptions
	waitOptions  ssa.WaitOptions
}

func applyInstanceInit(ctx context.Context, log logr.Logger, builder *engine.ModuleBuilder, buildResult cue.Value, instance *engine.BundleInstance, opts Options, timeout time.Duration) (*InstanceApplier, error) {
	applier := &InstanceApplier{
		opts:                 opts,
		isStandaloneInstance: instance.Bundle == "",
		currentObjects:       []*unstructured.Unstructured{},
		staleObjects:         []*unstructured.Unstructured{},
		deletedObjects:       []*unstructured.Unstructured{},
		applyOptions:         runtime.ApplyOptions(opts.Force, timeout),
		waitOptions: ssa.WaitOptions{
			Interval: 5 * time.Second,
			Timeout:  timeout,
			FailFast: true,
		},
	}
	applier.applyOptions.WaitInterval = applier.waitOptions.Interval

	if opts.DiffOutput == nil {
		opts.DiffOutput = io.Discard
	}

	if opts.ProgressStart == nil {
		opts.ProgressStart = func(msg string) ProgressStopper {
			log.Info(msg)
			return &noopProgressStopper{}
		}
	}

	finalValues, err := builder.GetDefaultValues()
	if err != nil {
		return nil, fmt.Errorf("failed to extract values: %w", err)
	}

	applier.sets, err = builder.GetApplySets(buildResult)
	if err != nil {
		return nil, fmt.Errorf("failed to extract objects: %w", err)
	}

	for _, set := range applier.sets {
		applier.currentObjects = append(applier.currentObjects, set.Objects...)
	}

	rm, err := runtime.NewResourceManager(opts.KubeConfigFlags)
	if err != nil {
		return nil, err
	}

	rm.SetOwnerLabels(applier.currentObjects, instance.Name, instance.Namespace)

	applier.storageManager = runtime.NewStorageManager(rm)
	storedInstance, err := applier.storageManager.Get(ctx, instance.Name, instance.Namespace)
	if err == nil {
		applier.instanceExists = true
	}

	applier.namespaceExists, err = applier.storageManager.NamespaceExists(ctx, instance.Namespace)
	if err != nil {
		return nil, fmt.Errorf("instance init failed: %w", err)
	}

	if !opts.OverwriteOwnership && applier.instanceExists && applier.isStandaloneInstance {
		if currentOwnerBundle := storedInstance.Labels[apiv1.BundleNameLabelKey]; currentOwnerBundle != "" {
			return nil, InstanceOwnershipConflictsErr(fmt.Sprintf("instance \"%s\" exists and is managed by bundle \"%s\"", instance.Name, currentOwnerBundle), "")
		}
	}

	applier.instanceManager = runtime.NewInstanceManager(instance.Name, instance.Namespace, finalValues, instance.Module)

	if !applier.isStandaloneInstance {
		if applier.instanceManager.Instance.Labels == nil {
			applier.instanceManager.Instance.Labels = make(map[string]string)
		}
		applier.instanceManager.Instance.Labels[apiv1.BundleNameLabelKey] = instance.Bundle
	}

	if err := applier.instanceManager.AddObjects(applier.currentObjects); err != nil {
		return nil, fmt.Errorf("adding objects to instance failed: %w", err)
	}

	applier.staleObjects, err = applier.storageManager.GetStaleObjects(ctx, &applier.instanceManager.Instance)
	if err != nil {
		return nil, fmt.Errorf("getting stale objects failed: %w", err)
	}
	return applier, nil
}

func ApplyInstance(ctx context.Context, log logr.Logger, builder *engine.ModuleBuilder, buildResult cue.Value, instance *engine.BundleInstance, opts Options, timeout time.Duration) error {
	applier, err := applyInstanceInit(ctx, log, builder, buildResult, instance, opts, timeout)
	if err != nil {
		return err
	}

	if opts.DryRun || opts.Diff {
		if !applier.namespaceExists {
			log.Info(logger.ColorizeJoin(logger.ColorizeSubject("Namespace/"+instance.Namespace),
				ssa.CreatedAction, logger.DryRunServer))
		}
		if err := applier.DryRunDiff(logr.NewContext(ctx, log)); err != nil {
			return err
		}

		log.Info(logger.ColorizeJoin("applied successfully", logger.ColorizeDryRun("(server dry run)")))
		return nil
	}

	if !applier.instanceExists {
		log.Info(fmt.Sprintf("installing %s in namespace %s",
			logger.ColorizeSubject(instance.Name), logger.ColorizeSubject(instance.Namespace)))

		if err := applier.storageManager.Apply(ctx, &applier.instanceManager.Instance, true); err != nil {
			return fmt.Errorf("instance init failed: %w", err)
		}

		if !applier.namespaceExists {
			log.Info(logger.ColorizeJoin(logger.ColorizeSubject("Namespace/"+instance.Namespace), ssa.CreatedAction))
		}
	} else {
		log.Info(fmt.Sprintf("upgrading %s in namespace %s",
			logger.ColorizeSubject(instance.Name), logger.ColorizeSubject(instance.Namespace)))
	}

	for _, set := range applier.sets {
		if len(applier.sets) > 1 {
			log.Info(fmt.Sprintf("applying %s", set.Name))
		}

		cs, err := applier.ApplyAllStaged(ctx, set)
		if err != nil {
			return err
		}
		for _, change := range cs.Entries {
			log.Info(logger.ColorizeJoin(change))
		}

		if opts.Wait {
			log.Info(fmt.Sprintf("%s resources %s", set.Name, logger.ColorizeReady("ready")))
			if err := applier.Wait(ctx, set); err != nil {
				return err
			}
			log.Info(fmt.Sprintf("%s resources %s", set.Name, logger.ColorizeReady("ready")))
		}
	}

	if images, err := builder.GetContainerImages(buildResult); err == nil {
		applier.instanceManager.Instance.Images = images
	}

	if err := applier.storageManager.Apply(ctx, &applier.instanceManager.Instance, true); err != nil {
		return fmt.Errorf("storing instance failed: %w", err)
	}

	if len(applier.staleObjects) > 0 {
		deleteOpts := runtime.DeleteOptions(instance.Name, instance.Namespace)
		changeSet, err := applier.resourceManager.DeleteAll(ctx, applier.staleObjects, deleteOpts)
		if err != nil {
			return fmt.Errorf("pruning objects failed: %w", err)
		}
		applier.deletedObjects = runtime.SelectObjectsFromSet(changeSet, ssa.DeletedAction)
		for _, change := range changeSet.Entries {
			log.Info(logger.ColorizeJoin(change))
		}
	}

	if opts.Wait {
		if len(applier.deletedObjects) > 0 {
			progress := opts.ProgressStart(fmt.Sprintf("waiting for %v resource(s) to be finalized...", len(deletedObjects)))
			err = applier.resourceManager.WaitForTermination(applier.deletedObjects, waitOptions)
			progress.Stop()
			if err != nil {
				return fmt.Errorf("waiting for termination failed: %w", err)
			}
		}
	}

	return nil
}

func InstanceOwnershipConflictsErr(description, hint string) error {
	msg := "instance ownership conflict encountered."
	if hint != "" {
		msg += " " + hint
	}
	msg += " Conflict: " + description
	return errors.New(msg)
}

func (a *InstanceApplier) ApplyAllStaged(ctx context.Context, set engine.ResourceSet) (*ssa.ChangeSet, error) {
	return a.resourceManager.ApplyAllStaged(ctx, set.Objects, a.applyOptions)
}

func (a *InstanceApplier) Wait(ctx context.Context, set engine.ResourceSet) error {
	progress := a.opts.ProgressStart(fmt.Sprintf("waiting for %v resource(s) to become ready...", len(set.Objects)))
	err := a.resourceManager.Wait(set.Objects, a.waitOptions)
	progress.Stop()
	return err
}

func (a *InstanceApplier) DryRunDiff(ctx context.Context) error {
	return dyff.InstanceDryRunDiff(
		ctx,
		a.resourceManager,
		a.currentObjects,
		a.staleObjects,
		a.namespaceExists,
		a.opts.Dir,
		a.opts.Diff,
		a.opts.DiffOutput,
	)
}
