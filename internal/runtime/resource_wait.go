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
	"fmt"

	pollingEngine "github.com/fluxcd/cli-utils/pkg/kstatus/polling/engine"
	"github.com/fluxcd/cli-utils/pkg/kstatus/polling/event"
	kstatusreaders "github.com/fluxcd/cli-utils/pkg/kstatus/polling/statusreaders"
	"github.com/fluxcd/cli-utils/pkg/kstatus/status"
	"github.com/fluxcd/cli-utils/pkg/object"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/stefanprodan/timoni/internal/engine"
)

// StatusReaderFactory creates a kstatus StatusReader for the given REST mapper.
type StatusReaderFactory func(mapper meta.RESTMapper) pollingEngine.StatusReader

type customHealthStatusReader struct {
	genericStatusReader pollingEngine.StatusReader
	lookup              func(gk schema.GroupKind) *engine.HealthCheck
}

// NewCustomHealthStatusReader returns a factory for a status reader that
// evaluates the module-defined CUE health checks on the live objects
// matching their GroupKind. A check with an exact GroupKind match takes
// precedence over a group-wide one (empty Kind).
func NewCustomHealthStatusReader(checks []*engine.HealthCheck) StatusReaderFactory {
	byGK := make(map[schema.GroupKind]*engine.HealthCheck, len(checks))
	for _, hc := range checks {
		byGK[hc.GroupKind] = hc
	}

	lookup := func(gk schema.GroupKind) *engine.HealthCheck {
		if hc, found := byGK[gk]; found {
			return hc
		}
		if hc, found := byGK[schema.GroupKind{Group: gk.Group}]; found {
			return hc
		}
		return nil
	}

	statusFunc := func(u *unstructured.Unstructured) (*status.Result, error) {
		hc := lookup(u.GroupVersionKind().GroupKind())
		if hc == nil {
			// Unreachable as Supports gates the GroupKinds,
			// fall back to the kstatus computation.
			return status.Compute(u)
		}
		return healthCheckResult(hc, u)
	}

	return func(mapper meta.RESTMapper) pollingEngine.StatusReader {
		return &customHealthStatusReader{
			genericStatusReader: kstatusreaders.NewGenericStatusReader(mapper, statusFunc),
			lookup:              lookup,
		}
	}
}

func (h *customHealthStatusReader) Supports(gk schema.GroupKind) bool {
	return h.lookup(gk) != nil
}

func (h *customHealthStatusReader) ReadStatus(ctx context.Context, reader pollingEngine.ClusterReader, resource object.ObjMetadata) (*event.ResourceStatus, error) {
	return h.genericStatusReader.ReadStatus(ctx, reader, resource)
}

func (h *customHealthStatusReader) ReadStatusForObject(ctx context.Context, reader pollingEngine.ClusterReader, resource *unstructured.Unstructured) (*event.ResourceStatus, error) {
	return h.genericStatusReader.ReadStatusForObject(ctx, reader, resource)
}

// healthCheckResult maps a health check evaluation to a kstatus result.
func healthCheckResult(hc *engine.HealthCheck, u *unstructured.Unstructured) (*status.Result, error) {
	// A resource scheduled for deletion is never ready, no matter
	// what its lingering status reports.
	if u.GetDeletionTimestamp() != nil {
		return &status.Result{
			Status:     status.TerminatingStatus,
			Message:    "Resource scheduled for deletion",
			Conditions: []status.Condition{},
		}, nil
	}

	res, err := hc.Evaluate(u.UnstructuredContent())
	if err != nil {
		return nil, err
	}

	switch res {
	case engine.HealthStatusCurrent:
		return &status.Result{
			Status:     status.CurrentStatus,
			Message:    fmt.Sprintf("Health check %q passed", hc.Name),
			Conditions: []status.Condition{},
		}, nil
	case engine.HealthStatusFailed:
		message := fmt.Sprintf("Health check %q failed", hc.Name)
		return &status.Result{
			Status:  status.FailedStatus,
			Message: message,
			Conditions: []status.Condition{
				{
					Type:    status.ConditionStalled,
					Status:  corev1.ConditionTrue,
					Reason:  "HealthCheckFailed",
					Message: message,
				},
			},
		}, nil
	default:
		message := fmt.Sprintf("Health check %q in progress", hc.Name)
		return &status.Result{
			Status:  status.InProgressStatus,
			Message: message,
			Conditions: []status.Condition{
				{
					Type:    status.ConditionReconciling,
					Status:  corev1.ConditionTrue,
					Reason:  "HealthCheckInProgress",
					Message: message,
				},
			},
		}, nil
	}
}
