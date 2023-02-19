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

package main

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestDelete(t *testing.T) {
	modPath := "testdata/cs"
	name := rnd("my-instance", 5)
	namespace := rnd("my-namespace", 5)

	t.Run("sets prune disabled annotation", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand(fmt.Sprintf(
			"apply -n %s %s %s -f %s -p main --wait",
			namespace,
			name,
			modPath,
			modPath+"-values/skip-prune.cue",
		))
		g.Expect(err).ToNot(HaveOccurred())

		clientCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-client", name),
				Namespace: namespace,
			},
		}

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(clientCM.GetAnnotations()).To(HaveKeyWithValue(apiv1.PruneAction, apiv1.DisabledValue))
	})

	t.Run("skips annotated resources on uninstall", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand(fmt.Sprintf(
			"delete -n %s %s --wait",
			namespace,
			name,
		))
		g.Expect(err).ToNot(HaveOccurred())

		clientCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-client", name),
				Namespace: namespace,
			},
		}

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
		g.Expect(err).ToNot(HaveOccurred())
	})
}
