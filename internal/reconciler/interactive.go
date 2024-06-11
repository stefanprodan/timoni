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
	"io"
	"time"

	"cuelang.org/go/cue"
	"github.com/fluxcd/pkg/ssa"
	"github.com/go-logr/logr"
	kerrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/stefanprodan/timoni/internal/dyff"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/logger"
)

func NewInteractiveReconciler(log logr.Logger, copts *CommonOptions, iopts *InteractiveOptions, timeout time.Duration) *InteractiveReconciler {
	reconciler := &InteractiveReconciler{
		Reconciler:         NewReconciler(log, copts, timeout),
		InteractiveOptions: iopts,
	}

	if reconciler.DiffOutput == nil {
		reconciler.DiffOutput = io.Discard
	}

	if iopts.ProgressStart != nil {
		reconciler.progressStartFn = iopts.ProgressStart
	}

	return reconciler
}

func (r *InteractiveReconciler) ApplyInstance(ctx context.Context, log logr.Logger, builder *engine.ModuleBuilder, buildResult cue.Value) error {
	namespaceExists, err := r.NamespaceExists(ctx)
	if err != nil {
		return err
	}

	if r.DryRun || r.Diff {
		if !namespaceExists {
			log.Info(logger.ColorizeJoin(logger.ColorizeSubject("Namespace/"+r.Namespace()),
				ssa.CreatedAction, logger.DryRunServer))
		}
		if err := r.DryRunDiff(logr.NewContext(ctx, log), namespaceExists); err != nil {
			return err
		}

		log.Info(logger.ColorizeJoin("applied successfully", logger.ColorizeDryRun("(server dry run)")))
		return nil
	}

	if !r.instanceExists {
		log.Info(fmt.Sprintf("installing %s in namespace %s",
			logger.ColorizeSubject(r.Name()), logger.ColorizeSubject(r.Namespace())))

		if err := r.UpdateStoredInstance(ctx); err != nil {
			return fmt.Errorf("instance init failed: %w", err)
		}

		if !namespaceExists {
			log.Info(logger.ColorizeJoin(logger.ColorizeSubject("Namespace/"+r.Namespace()), ssa.CreatedAction))
		}
	} else {
		log.Info(fmt.Sprintf("upgrading %s in namespace %s",
			logger.ColorizeSubject(r.Name()), logger.ColorizeSubject(r.Namespace())))
	}

	return kerrors.NewAggregate([]error{
		r.ApplyAllSets(ctx, log, r.Wait),
		r.PostApplyUpdateInventory(ctx, builder, buildResult),
		r.PostApplyPruneStaleObjects(ctx, log, r.WaitForTermination),
	})
}

func (r *InteractiveReconciler) DryRunDiff(ctx context.Context, namespaceExists bool) error {
	return dyff.InstanceDryRunDiff(
		ctx,
		r.resourceManager,
		r.currentObjects,
		r.staleObjects,
		namespaceExists,
		r.opts.Dir,
		r.Diff,
		r.DiffOutput,
	)
}

func (r *InteractiveReconciler) Wait(ctx context.Context, log logr.Logger, cs *ssa.ChangeSet, rs *engine.ResourceSet) error {
	for _, change := range cs.Entries {
		log.Info(logger.ColorizeJoin(change))
	}
	doneMsg := ""
	if rs != nil && rs.Name != "" {
		doneMsg = fmt.Sprintf("%s resources %s", rs.Name, logger.ColorizeReady("ready"))
	}
	return r.doWait(ctx, log, rs, "waiting for %d resource(s) to become ready...", doneMsg)
}

func (r *InteractiveReconciler) WaitForTermination(ctx context.Context, log logr.Logger, cs *ssa.ChangeSet, _ *engine.ResourceSet) error {
	for _, change := range cs.Entries {
		log.Info(logger.ColorizeJoin(change))
	}
	return r.doWaitForTermination(ctx, log, cs, "waiting for %d resource(s) to be finalized...")
}
