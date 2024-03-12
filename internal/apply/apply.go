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

func ApplyInstance(ctx context.Context, log logr.Logger, builder *engine.ModuleBuilder, buildResult cue.Value, instance *engine.BundleInstance, opts Options, timeout time.Duration) error {
	isStandaloneInstance := instance.Bundle == ""

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
		return fmt.Errorf("failed to extract values: %w", err)
	}

	sets, err := builder.GetApplySets(buildResult)
	if err != nil {
		return fmt.Errorf("failed to extract objects: %w", err)
	}

	var objects []*unstructured.Unstructured
	for _, set := range sets {
		objects = append(objects, set.Objects...)
	}

	rm, err := runtime.NewResourceManager(opts.KubeConfigFlags)
	if err != nil {
		return err
	}

	rm.SetOwnerLabels(objects, instance.Name, instance.Namespace)

	exists := false
	sm := runtime.NewStorageManager(rm)
	storedInstance, err := sm.Get(ctx, instance.Name, instance.Namespace)
	if err == nil {
		exists = true
	}

	nsExists, err := sm.NamespaceExists(ctx, instance.Namespace)
	if err != nil {
		return fmt.Errorf("instance init failed: %w", err)
	}

	if !opts.OverwriteOwnership && exists && isStandaloneInstance {
		if currentOwnerBundle := storedInstance.Labels[apiv1.BundleNameLabelKey]; currentOwnerBundle != "" {
			return InstanceOwnershipConflictsErr(fmt.Sprintf("instance \"%s\" exists and is managed by bundle \"%s\"", instance.Name, currentOwnerBundle), "")
		}
	}

	im := runtime.NewInstanceManager(instance.Name, instance.Namespace, finalValues, instance.Module)

	if !isStandaloneInstance {
		if im.Instance.Labels == nil {
			im.Instance.Labels = make(map[string]string)
		}
		im.Instance.Labels[apiv1.BundleNameLabelKey] = instance.Bundle
	}

	if err := im.AddObjects(objects); err != nil {
		return fmt.Errorf("adding objects to instance failed: %w", err)
	}

	staleObjects, err := sm.GetStaleObjects(ctx, &im.Instance)
	if err != nil {
		return fmt.Errorf("getting stale objects failed: %w", err)
	}

	if opts.DryRun || opts.Diff {
		if !nsExists {
			log.Info(logger.ColorizeJoin(logger.ColorizeSubject("Namespace/"+instance.Namespace),
				ssa.CreatedAction, logger.DryRunServer))
		}
		if err := dyff.InstanceDryRunDiff(
			logr.NewContext(ctx, log),
			rm,
			objects,
			staleObjects,
			nsExists,
			opts.Dir,
			opts.Diff,
			opts.DiffOutput,
		); err != nil {
			return err
		}

		log.Info(logger.ColorizeJoin("applied successfully", logger.ColorizeDryRun("(server dry run)")))
		return nil
	}

	if !exists {
		log.Info(fmt.Sprintf("installing %s in namespace %s",
			logger.ColorizeSubject(instance.Name), logger.ColorizeSubject(instance.Namespace)))

		if err := sm.Apply(ctx, &im.Instance, true); err != nil {
			return fmt.Errorf("instance init failed: %w", err)
		}

		if !nsExists {
			log.Info(logger.ColorizeJoin(logger.ColorizeSubject("Namespace/"+instance.Namespace), ssa.CreatedAction))
		}
	} else {
		log.Info(fmt.Sprintf("upgrading %s in namespace %s",
			logger.ColorizeSubject(instance.Name), logger.ColorizeSubject(instance.Namespace)))
	}

	applyOpts := runtime.ApplyOptions(opts.Force, timeout)
	applyOpts.WaitInterval = 5 * time.Second

	waitOptions := ssa.WaitOptions{
		Interval: applyOpts.WaitInterval,
		Timeout:  timeout,
		FailFast: true,
	}

	for _, set := range sets {
		if len(sets) > 1 {
			log.Info(fmt.Sprintf("applying %s", set.Name))
		}

		cs, err := rm.ApplyAllStaged(ctx, set.Objects, applyOpts)
		if err != nil {
			return err
		}
		for _, change := range cs.Entries {
			log.Info(logger.ColorizeJoin(change))
		}

		if opts.Wait {
			progress := opts.ProgressStart(fmt.Sprintf("waiting for %v resource(s) to become ready...", len(set.Objects)))
			err = rm.Wait(set.Objects, waitOptions)
			progress.Stop()
			if err != nil {
				return err
			}
			log.Info(fmt.Sprintf("%s resources %s", set.Name, logger.ColorizeReady("ready")))
		}
	}

	if images, err := builder.GetContainerImages(buildResult); err == nil {
		im.Instance.Images = images
	}

	if err := sm.Apply(ctx, &im.Instance, true); err != nil {
		return fmt.Errorf("storing instance failed: %w", err)
	}

	var deletedObjects []*unstructured.Unstructured
	if len(staleObjects) > 0 {
		deleteOpts := runtime.DeleteOptions(instance.Name, instance.Namespace)
		changeSet, err := rm.DeleteAll(ctx, staleObjects, deleteOpts)
		if err != nil {
			return fmt.Errorf("pruning objects failed: %w", err)
		}
		deletedObjects = runtime.SelectObjectsFromSet(changeSet, ssa.DeletedAction)
		for _, change := range changeSet.Entries {
			log.Info(logger.ColorizeJoin(change))
		}
	}

	if opts.Wait {
		if len(deletedObjects) > 0 {
			progress := opts.ProgressStart(fmt.Sprintf("waiting for %v resource(s) to be finalized...", len(deletedObjects)))
			err = rm.WaitForTermination(deletedObjects, waitOptions)
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
