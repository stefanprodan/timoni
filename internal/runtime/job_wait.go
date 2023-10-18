/*
Copyright 2023 Stefan Prodan

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

/*
Derived work from:
https://github.com/fluxcd/kustomize-controller/tree/v1.1.1/internal/statusreaders
https://github.com/kubernetes-sigs/cli-utils/blob/0b156cb0425fdb29a436d13f840a39039558c10e/pkg/kstatus/status/core.go#L533
*/

package runtime

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/engine"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling/event"
	kstatusreaders "sigs.k8s.io/cli-utils/pkg/kstatus/polling/statusreaders"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
	"sigs.k8s.io/cli-utils/pkg/object"
)

type customJobStatusReader struct {
	genericStatusReader engine.StatusReader
}

// NewCustomJobStatusReader creates a reader that asserts a Job completion status.
func NewCustomJobStatusReader(mapper meta.RESTMapper) engine.StatusReader {
	genericStatusReader := kstatusreaders.NewGenericStatusReader(mapper, jobConditions)
	return &customJobStatusReader{
		genericStatusReader: genericStatusReader,
	}
}

func (j *customJobStatusReader) Supports(gk schema.GroupKind) bool {
	return gk == batchv1.SchemeGroupVersion.WithKind("Job").GroupKind()
}

func (j *customJobStatusReader) ReadStatus(ctx context.Context, reader engine.ClusterReader, resource object.ObjMetadata) (*event.ResourceStatus, error) {
	return j.genericStatusReader.ReadStatus(ctx, reader, resource)
}

func (j *customJobStatusReader) ReadStatusForObject(ctx context.Context, reader engine.ClusterReader, resource *unstructured.Unstructured) (*event.ResourceStatus, error) {
	return j.genericStatusReader.ReadStatusForObject(ctx, reader, resource)
}

func jobConditions(u *unstructured.Unstructured) (*status.Result, error) {
	obj := u.UnstructuredContent()

	parallelism := status.GetIntField(obj, ".spec.parallelism", 1)
	completions := status.GetIntField(obj, ".spec.completions", parallelism)
	active := status.GetIntField(obj, ".status.active", 1)
	succeeded := status.GetIntField(obj, ".status.succeeded", 0)
	failed := status.GetIntField(obj, ".status.failed", 0)

	objc, err := status.GetObjectWithConditions(obj)
	if err != nil {
		return nil, err
	}
	for _, c := range objc.Status.Conditions {
		switch c.Type {
		case "Complete":
			if c.Status == corev1.ConditionTrue {
				message := fmt.Sprintf("Job Completed. succeeded: %d/%d", succeeded, completions)
				return &status.Result{
					Status:     status.CurrentStatus,
					Message:    message,
					Conditions: []status.Condition{},
				}, nil
			}
		case "Failed":
			message := fmt.Sprintf("Job Failed. failed: %d/%d error: %s", failed, completions, c.Message)
			if c.Status == corev1.ConditionTrue {
				return &status.Result{
					Status:  status.FailedStatus,
					Message: message,
					Conditions: []status.Condition{
						{
							Type:    status.ConditionStalled,
							Status:  corev1.ConditionTrue,
							Reason:  "JobFailed",
							Message: c.Message,
						},
					},
				}, nil
			}
		}
	}

	message := fmt.Sprintf("Job in progress. active: %d", active)
	return &status.Result{
		Status:  status.InProgressStatus,
		Message: message,
		Conditions: []status.Condition{
			{
				Type:    status.ConditionReconciling,
				Status:  corev1.ConditionTrue,
				Reason:  "JobInProgress",
				Message: message,
			},
		},
	}, nil
}
