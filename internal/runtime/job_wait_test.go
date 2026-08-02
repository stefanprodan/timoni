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
https://github.com/fluxcd/pkg/blob/runtime/v0.111.0/runtime/statusreaders/job_test.go
*/

package runtime

import (
	"testing"

	"github.com/fluxcd/cli-utils/pkg/kstatus/status"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Test_jobConditions(t *testing.T) {
	t.Run("job without Complete condition returns InProgress status", func(t *testing.T) {
		g := NewWithT(t)
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: "job",
			},
			Spec:   batchv1.JobSpec{},
			Status: batchv1.JobStatus{},
		}
		us, err := ToUnstructured(job)
		g.Expect(err).ToNot(HaveOccurred())
		result, err := jobConditions(us)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Status).To(Equal(status.InProgressStatus))
	})

	t.Run("job with Complete condition as True returns Current status", func(t *testing.T) {
		g := NewWithT(t)
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: "job",
			},
			Spec: batchv1.JobSpec{},
			Status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobComplete,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		us, err := ToUnstructured(job)
		g.Expect(err).ToNot(HaveOccurred())
		result, err := jobConditions(us)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Status).To(Equal(status.CurrentStatus))
		g.Expect(result.Message).To(ContainSubstring("Job Completed"))
	})

	t.Run("suspended job returns Current status", func(t *testing.T) {
		g := NewWithT(t)
		suspend := true
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: "job",
			},
			Spec: batchv1.JobSpec{
				Suspend: &suspend,
			},
			Status: batchv1.JobStatus{},
		}
		us, err := ToUnstructured(job)
		g.Expect(err).ToNot(HaveOccurred())
		result, err := jobConditions(us)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Status).To(Equal(status.CurrentStatus))
		g.Expect(result.Message).To(Equal("Job is suspended"))
	})

	t.Run("job with Failed condition as True returns Failed status", func(t *testing.T) {
		g := NewWithT(t)
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: "job",
			},
			Spec: batchv1.JobSpec{},
			Status: batchv1.JobStatus{
				Failed: 1,
				Conditions: []batchv1.JobCondition{
					{
						Type:    batchv1.JobFailed,
						Status:  corev1.ConditionTrue,
						Message: "Job has reached the specified backoff limit",
					},
				},
			},
		}
		us, err := ToUnstructured(job)
		g.Expect(err).ToNot(HaveOccurred())
		result, err := jobConditions(us)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Status).To(Equal(status.FailedStatus))
		g.Expect(result.Message).To(ContainSubstring("Job Failed"))
		g.Expect(result.Message).To(ContainSubstring("error: Job has reached the specified backoff limit"))
		g.Expect(result.Conditions).To(HaveLen(1))
		g.Expect(result.Conditions[0].Type).To(Equal(status.ConditionStalled))
		g.Expect(result.Conditions[0].Reason).To(Equal("JobFailed"))
	})

	t.Run("job not started returns InProgress status", func(t *testing.T) {
		g := NewWithT(t)
		parallelism := int32(3)
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: "job",
			},
			Spec: batchv1.JobSpec{
				Parallelism: &parallelism,
			},
			Status: batchv1.JobStatus{},
		}
		us, err := ToUnstructured(job)
		g.Expect(err).ToNot(HaveOccurred())
		result, err := jobConditions(us)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Status).To(Equal(status.InProgressStatus))
		g.Expect(result.Message).To(Equal("Job not started. active: 0/3"))
		g.Expect(result.Conditions).To(HaveLen(1))
		g.Expect(result.Conditions[0].Type).To(Equal(status.ConditionReconciling))
		g.Expect(result.Conditions[0].Reason).To(Equal("JobNotStarted"))
	})

	t.Run("job without status field returns InProgress status", func(t *testing.T) {
		g := NewWithT(t)
		us := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "batch/v1",
				"kind":       "Job",
				"metadata": map[string]any{
					"name": "job",
				},
				"spec": map[string]any{
					"parallelism": int64(2),
				},
			},
		}
		result, err := jobConditions(us)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Status).To(Equal(status.InProgressStatus))
		g.Expect(result.Message).To(Equal("Job not started. active: 0/2"))
	})

	t.Run("job in progress returns InProgress status", func(t *testing.T) {
		g := NewWithT(t)
		now := metav1.Now()
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: "job",
			},
			Spec: batchv1.JobSpec{},
			Status: batchv1.JobStatus{
				StartTime: &now,
				Active:    2,
				Succeeded: 1,
				Failed:    1,
			},
		}
		us, err := ToUnstructured(job)
		g.Expect(err).ToNot(HaveOccurred())
		result, err := jobConditions(us)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Status).To(Equal(status.InProgressStatus))
		g.Expect(result.Message).To(Equal("Job in progress. success:1, active: 2, failed: 1"))
		g.Expect(result.Conditions).To(HaveLen(1))
		g.Expect(result.Conditions[0].Type).To(Equal(status.ConditionReconciling))
		g.Expect(result.Conditions[0].Reason).To(Equal("JobInProgress"))
	})
}
