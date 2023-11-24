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
*/

package runtime

import (
	"testing"

	"github.com/fluxcd/cli-utils/pkg/kstatus/status"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_jobConditions(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "job",
		},
		Spec:   batchv1.JobSpec{},
		Status: batchv1.JobStatus{},
	}

	t.Run("job without Complete condition returns InProgress status", func(t *testing.T) {
		g := NewWithT(t)
		us, err := ToUnstructured(job)
		g.Expect(err).ToNot(HaveOccurred())
		result, err := jobConditions(us)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Status).To(Equal(status.InProgressStatus))
	})

	t.Run("job with Complete condition as True returns Current status", func(t *testing.T) {
		g := NewWithT(t)
		job.Status = batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{
					Type:   batchv1.JobComplete,
					Status: corev1.ConditionTrue,
				},
			},
		}
		us, err := ToUnstructured(job)
		g.Expect(err).ToNot(HaveOccurred())
		result, err := jobConditions(us)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Status).To(Equal(status.CurrentStatus))
	})
}
