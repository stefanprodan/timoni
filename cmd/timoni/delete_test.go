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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestDelete(t *testing.T) {
	modPath := "testdata/module"
	name := rnd("my-instance")
	namespace := rnd("my-namespace")

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

func TestDelete_RetainsInventoryOnTerminationTimeout(t *testing.T) {
	modPath := "testdata/module"
	name := rnd("my-instance")
	namespace := rnd("my-namespace")
	g := NewWithT(t)

	_, err := executeCommand(fmt.Sprintf(
		"apply -n %s %s %s -p main --wait",
		namespace,
		name,
		modPath,
	))
	g.Expect(err).ToNot(HaveOccurred())

	clientCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-client", name),
			Namespace: namespace,
		},
	}
	g.Expect(envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)).ToNot(HaveOccurred())

	// Block termination with a finalizer that no controller will clear.
	clientCM.SetFinalizers(append(clientCM.GetFinalizers(), "timoni.test/finalizer"))
	g.Expect(envTestClient.Update(context.Background(), clientCM)).ToNot(HaveOccurred())

	// The delete requests succeed, but the termination wait times out.
	output, err := executeCommand(fmt.Sprintf(
		"delete -n %s %s --wait --timeout=5s",
		namespace,
		name,
	))
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("termination timeout"))
	t.Log("\n", output)

	// The instance record must survive so the delete can be retried.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s.%s", apiv1.FieldManager, name),
			Namespace: namespace,
		},
	}
	g.Expect(envTestClient.Get(context.Background(), client.ObjectKeyFromObject(secret), secret)).ToNot(HaveOccurred())
	g.Expect(secret.GetAnnotations()).To(HaveKey(apiv1.DeleteInProgressAnnotation))
	instanceData := string(secret.Data[strings.ToLower(apiv1.InstanceKind)])
	g.Expect(instanceData).To(ContainSubstring(fmt.Sprintf("%s-client", name)))
	g.Expect(instanceData).To(ContainSubstring(fmt.Sprintf("%s-server", name)))

	// The blocked object is still present while the unblocked one is gone.
	err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clientCM.GetDeletionTimestamp()).ToNot(BeNil())

	serverCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-server", name),
			Namespace: namespace,
		},
	}
	err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(serverCM), serverCM)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())

	// Resume: once the finalizer clears, retrying the delete finishes the
	// job and removes the record.
	clientCM.SetFinalizers(nil)
	g.Expect(envTestClient.Update(context.Background(), clientCM)).ToNot(HaveOccurred())

	_, err = executeCommand(fmt.Sprintf(
		"delete -n %s %s --wait",
		namespace,
		name,
	))
	g.Expect(err).ToNot(HaveOccurred())

	err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(secret), secret)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
}

func TestDelete_WaitFalseRemovesStateWithoutWaiting(t *testing.T) {
	modPath := "testdata/module"
	name := rnd("my-instance")
	namespace := rnd("my-namespace")
	g := NewWithT(t)

	_, err := executeCommand(fmt.Sprintf(
		"apply -n %s %s %s -p main --wait",
		namespace,
		name,
		modPath,
	))
	g.Expect(err).ToNot(HaveOccurred())

	clientCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-client", name),
			Namespace: namespace,
		},
	}
	g.Expect(envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)).ToNot(HaveOccurred())
	clientCM.SetFinalizers(append(clientCM.GetFinalizers(), "timoni.test/finalizer"))
	g.Expect(envTestClient.Update(context.Background(), clientCM)).ToNot(HaveOccurred())

	// Like kubectl, --wait=false sends the delete requests and returns right
	// away, without waiting for finalizers, and removes the instance record.
	_, err = executeCommand(fmt.Sprintf(
		"delete -n %s %s --wait=false",
		namespace,
		name,
	))
	g.Expect(err).ToNot(HaveOccurred())

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s.%s", apiv1.FieldManager, name),
			Namespace: namespace,
		},
	}
	err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(secret), secret)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())

	// The finalizer is still blocking the object: nothing was waited on.
	err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clientCM.GetDeletionTimestamp()).ToNot(BeNil())

	// Once the finalizer clears, the pending deletion completes on its own.
	clientCM.SetFinalizers(nil)
	g.Expect(envTestClient.Update(context.Background(), clientCM)).ToNot(HaveOccurred())
	g.Eventually(func() bool {
		err := envTestClient.Get(context.Background(), client.ObjectKeyFromObject(clientCM), clientCM)
		return apierrors.IsNotFound(err)
	}, "10s", "500ms").Should(BeTrue())
}

func TestDelete_RemovesStateWhenNothingDeleted(t *testing.T) {
	modPath := "testdata/module"
	name := rnd("my-instance")
	namespace := rnd("my-namespace")
	g := NewWithT(t)

	// Every object carries the prune-disabled annotation, so the delete skips
	// all of them and no deletion is requested.
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
	serverCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-server", name),
			Namespace: namespace,
		},
	}
	for _, cm := range []*corev1.ConfigMap{clientCM, serverCM} {
		g.Expect(envTestClient.Get(context.Background(), client.ObjectKeyFromObject(cm), cm)).ToNot(HaveOccurred())
		g.Expect(cm.GetAnnotations()).To(HaveKeyWithValue(apiv1.PruneAction, apiv1.DisabledValue))
	}

	_, err = executeCommand(fmt.Sprintf(
		"delete -n %s %s --wait",
		namespace,
		name,
	))
	g.Expect(err).ToNot(HaveOccurred())

	// Nothing was deleted, so the instance record is removed right away and
	// the prune-disabled objects stay.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s.%s", apiv1.FieldManager, name),
			Namespace: namespace,
		},
	}
	err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(secret), secret)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())

	for _, cm := range []*corev1.ConfigMap{clientCM, serverCM} {
		err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(cm), cm)
		g.Expect(err).ToNot(HaveOccurred())
	}
}

func TestDelete_DoesNotTouchForeignSameLabelObjects(t *testing.T) {
	modPath := "testdata/module"
	tGroup := fmt.Sprintf("%s.%s", strings.ToLower(apiv1.InstanceKind), apiv1.GroupVersion.Group)
	name := rnd("my-instance")
	namespace := rnd("my-namespace")
	g := NewWithT(t)

	_, err := executeCommand(fmt.Sprintf(
		"apply -n %s %s %s -p main --wait",
		namespace,
		name,
		modPath,
	))
	g.Expect(err).ToNot(HaveOccurred())

	// A foreign object carrying the instance ownership labels but absent from
	// the inventory must never be adopted or deleted.
	foreign := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-foreign", name),
			Namespace: namespace,
			Labels: map[string]string{
				tGroup + "/name":      name,
				tGroup + "/namespace": namespace,
			},
		},
		Data: map[string]string{"keep": "me"},
	}
	g.Expect(envTestClient.Create(context.Background(), foreign)).ToNot(HaveOccurred())

	_, err = executeCommand(fmt.Sprintf(
		"delete -n %s %s --wait",
		namespace,
		name,
	))
	g.Expect(err).ToNot(HaveOccurred())

	err = envTestClient.Get(context.Background(), client.ObjectKeyFromObject(foreign), foreign)
	g.Expect(err).ToNot(HaveOccurred())
}
