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

package engine

import (
	"fmt"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"k8s.io/apimachinery/pkg/runtime/schema"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// HealthStatus is the outcome of a HealthCheck evaluation.
type HealthStatus string

const (
	// HealthStatusCurrent means the resource is ready.
	HealthStatusCurrent HealthStatus = "Current"

	// HealthStatusInProgress means the resource is still reconciling.
	HealthStatusInProgress HealthStatus = "InProgress"

	// HealthStatusFailed means the resource reached a terminal failure state.
	HealthStatusFailed HealthStatus = "Failed"
)

// HealthCheck holds a module-defined CUE readiness evaluation
// for the Kubernetes resources matching its GroupKind.
type HealthCheck struct {
	// Name is the health check's key under 'timoni: healthChecks:'.
	Name string

	// GroupKind of the target resources; an empty Kind makes the
	// check apply to every kind in the group.
	GroupKind schema.GroupKind

	value cue.Value
	mu    *sync.Mutex
}

// healthCheckExprs are the expression fields of a health check
// in evaluation order: the first one returning true wins.
var healthCheckExprs = []struct {
	path   string
	status HealthStatus
}{
	{"inProgress", HealthStatusInProgress},
	{"failed", HealthStatusFailed},
	{"current", HealthStatusCurrent},
}

// GetHealthChecks extracts the custom health checks declared by the module
// under 'timoni: healthChecks:'. It returns nil when the field is absent
// and errors on duplicate GroupKind targets.
func (b *ModuleBuilder) GetHealthChecks(value cue.Value) ([]*HealthCheck, error) {
	checks := value.LookupPath(cue.ParsePath(apiv1.HealthChecksSelector.String()))
	if !checks.Exists() {
		return nil, nil
	}
	if err := checks.Err(); err != nil {
		return nil, fmt.Errorf("lookup %s failed: %w", apiv1.HealthChecksSelector, err)
	}

	iter, err := checks.Fields()
	if err != nil {
		return nil, fmt.Errorf("reading %s failed: %w", apiv1.HealthChecksSelector, err)
	}

	var result []*HealthCheck
	seen := make(map[schema.GroupKind]string)
	// The CUE context is not safe for concurrent evaluation, so all
	// checks extracted from the same build share one lock, guarding
	// evaluations triggered by concurrent status polling.
	mu := &sync.Mutex{}

	for iter.Next() {
		name := iter.Selector().Unquoted()
		check := iter.Value()

		// Validate as final so that missing required fields
		// e.g. the current expression are reported.
		if err := check.Validate(cue.Final()); err != nil {
			return nil, fmt.Errorf("health check %q is invalid: %w", name, err)
		}

		group, err := check.LookupPath(cue.ParsePath("group")).String()
		if err != nil {
			return nil, fmt.Errorf("health check %q: reading group failed: %w", name, err)
		}

		// Guard against values bypassing the schema constraints: an empty
		// group would silently map to the core API group, which is covered
		// by kstatus and not a valid health check target.
		if group == "" || strings.Contains(group, "/") {
			return nil, fmt.Errorf("health check %q: group %q must be a custom resource API group e.g. 'cert-manager.io'", name, group)
		}

		var kind string
		if kv := check.LookupPath(cue.ParsePath("kind")); kv.Exists() {
			if kind, err = kv.String(); err != nil {
				return nil, fmt.Errorf("health check %q: reading kind failed: %w", name, err)
			}
		}

		gk := schema.GroupKind{Group: group, Kind: kind}
		if prev, found := seen[gk]; found {
			return nil, fmt.Errorf("health checks %q and %q target the same GroupKind %s", prev, name, gk)
		}
		seen[gk] = name

		result = append(result, &HealthCheck{
			Name:      name,
			GroupKind: gk,
			value:     check,
			mu:        mu,
		})
	}

	return result, nil
}

// Evaluate fills the health check's #object with the live object content
// and evaluates the inProgress, failed and current expressions in order,
// returning the status of the first expression yielding true, or
// HealthStatusInProgress when none does. Expressions that cannot be
// evaluated because the referenced fields are missing from the live
// object count as false; any other evaluation failure returns an error.
func (hc *HealthCheck) Evaluate(object map[string]any) (HealthStatus, error) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	val := hc.value.FillPath(cue.MakePath(cue.Def("#object")), object)
	if err := val.Err(); err != nil {
		return "", fmt.Errorf("health check %q: the object does not match the #object schema: %w", hc.Name, err)
	}

	for _, expr := range healthCheckExprs {
		v := val.LookupPath(cue.ParsePath(expr.path))
		if !v.Exists() {
			continue
		}

		res, err := v.Bool()
		if err != nil {
			if !v.IsConcrete() && v.Validate() == nil {
				// The expression is incomplete: it refers to fields the
				// live object does not have (yet), which counts as false.
				continue
			}
			return "", fmt.Errorf("health check %q: evaluating the %s expression failed: %w", hc.Name, expr.path, err)
		}

		if res {
			return expr.status, nil
		}
	}

	return HealthStatusInProgress, nil
}
