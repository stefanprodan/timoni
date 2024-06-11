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
	"strings"

	"github.com/fluxcd/pkg/ssa"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/stefanprodan/timoni/internal/engine"
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

	ProgressStart func(string) interface{ Stop() }
}

type Reconciler struct {
	opts *CommonOptions

	instanceExists bool

	sets []engine.ResourceSet

	currentObjects, staleObjects []*unstructured.Unstructured

	storageManager  *runtime.StorageManager
	instanceManager *runtime.InstanceManager
	resourceManager *ssa.ResourceManager

	applyOptions ssa.ApplyOptions
	waitOptions  ssa.WaitOptions

	progressStartFn func(string) interface{ Stop() }
}

type InteractiveReconciler struct {
	*Reconciler
	*InteractiveOptions
}

type noopProgressStopper struct{}

func (*noopProgressStopper) Stop() {}

type withChangeSetFunc func(context.Context, logr.Logger, *ssa.ChangeSet, *engine.ResourceSet) error

type InstanceOwnershipConflict struct{ InstanceName, CurrentOwnerBundle string }
type InstanceOwnershipConflictErr []InstanceOwnershipConflict

func (e *InstanceOwnershipConflictErr) Error() string {
	s := &strings.Builder{}
	s.WriteString("instance ownership conflict encountered. ")
	s.WriteString("Conflict: ")
	numConflicts := len(*e)
	for i, c := range *e {
		if c.CurrentOwnerBundle != "" {
			s.WriteString(fmt.Sprintf("instance %q exists and is managed by another bundle %q", c.InstanceName, c.CurrentOwnerBundle))
		} else {
			s.WriteString(fmt.Sprintf("instance %q exists and is not managed by any bundle", c.InstanceName))
		}
		if numConflicts > 1 && i != numConflicts {
			s.WriteString("; ")
		}
	}
	return s.String()
}
