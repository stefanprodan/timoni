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
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestApply(t *testing.T) {
	modPath := "testdata/cs"
	tGroup := fmt.Sprintf("%s.%s", strings.ToLower(apiv1.InstanceKind), apiv1.GroupVersion.Group)
	name := rnd("my-instance", 5)
	namespace := rnd("my-namespace", 5)

	t.Run("creates instance with default values", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"apply -n %s %s %s -p main --wait",
			namespace,
			name,
			modPath,
		))
		g.Expect(err).ToNot(HaveOccurred())
		t.Log("\n", output)

		clientCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-client", name),
				Namespace: namespace,
			},
		}

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(clientCM.GetLabels()).To(HaveKeyWithValue(tGroup+"/name", name))
		g.Expect(clientCM.GetLabels()).To(HaveKeyWithValue(tGroup+"/namespace", namespace))
	})

	t.Run("updates instance with custom values", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"apply -n %s %s %s -f %s -p main --wait",
			namespace,
			name,
			modPath,
			modPath+"-values/example.com.cue",
		))
		g.Expect(err).ToNot(HaveOccurred())
		t.Log("\n", output)

		clientCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-client", name),
				Namespace: namespace,
			},
		}

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(clientCM.GetAnnotations()).To(HaveKeyWithValue("scope", "external"))
		g.Expect(clientCM.Data["server"]).To(ContainSubstring("tcp://example.com"))
	})

	t.Run("prunes resources removed from instance", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"apply -n %s %s %s -f %s -f %s -f %s -p main --wait",
			namespace,
			name,
			modPath,
			modPath+"-values/example.com.cue",
			modPath+"-values/example.io.cue",
			modPath+"-values/server-only.cue",
		))
		g.Expect(err).ToNot(HaveOccurred())
		t.Log("\n", output)

		clientCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-client", name),
				Namespace: namespace,
			},
		}

		serverCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-server", name),
				Namespace: namespace,
			},
		}

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(serverCM), serverCM)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(serverCM.GetAnnotations()).To(HaveKeyWithValue("scope", "external"))
		g.Expect(serverCM.Data["hostname"]).To(BeEquivalentTo("example.io"))

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
		g.Expect(err).To(HaveOccurred())
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	t.Run("uninstalls instance", func(t *testing.T) {
		g := NewWithT(t)
		output, err := executeCommand(fmt.Sprintf(
			"delete -n %s %s --wait",
			namespace,
			name,
		))
		g.Expect(err).ToNot(HaveOccurred())
		t.Log("\n", output)

		serverCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-server", name),
				Namespace: namespace,
			},
		}

		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(serverCM), serverCM)
		g.Expect(err).To(HaveOccurred())
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
}

func TestApply_Actions(t *testing.T) {
	modPath := "testdata/cs"
	name := rnd("my-instance", 5)
	namespace := rnd("my-namespace", 5)

	t.Run("sets prune and force annotation", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand(fmt.Sprintf(
			"apply -n %s %s %s -f %s -f %s -p main --wait",
			namespace,
			name,
			modPath,
			modPath+"-values/skip-prune.cue",
			modPath+"-values/force-apply.cue",
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
		g.Expect(clientCM.GetAnnotations()).To(HaveKeyWithValue(apiv1.ForceAction, apiv1.EnabledValue))
	})

	t.Run("skips pruning resources removed from instance", func(t *testing.T) {
		g := NewWithT(t)
		_, err := executeCommand(fmt.Sprintf(
			"apply -n %s %s %s -f %s -f %s -p main --wait",
			namespace,
			name,
			modPath,
			modPath+"-values/skip-prune.cue",
			modPath+"-values/server-only.cue",
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
