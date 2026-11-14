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

package main

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/runtime"
)

func TestApply_RecreateAndDeleteAfterHooks(t *testing.T) {
	g := NewWithT(t)
	modPath := "testdata/module-hooks"
	name := rnd("hooks")
	namespace := rnd("hooks-ns")

	applyCmd := fmt.Sprintf("apply -n %s %s %s -p main --wait", namespace, name, modPath)

	_, err := executeCommand(applyCmd)
	g.Expect(err).ToNot(HaveOccurred())

	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name + "-sa", Namespace: namespace}}
	dsn := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name + "-dsn", Namespace: namespace}}
	migrate := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name + "-migrate", Namespace: namespace}}
	app := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name + "-app", Namespace: namespace}}
	post := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name + "-post", Namespace: namespace}}

	g.Expect(envTestClient.Get(context.Background(), client.ObjectKeyFromObject(sa), sa)).To(Succeed())
	g.Expect(sa.GetAnnotations()).To(HaveKeyWithValue(apiv1.RecreateAction, apiv1.EnabledValue))

	// delete-after-hooks objects are gone after the hooks group succeeds
	g.Expect(apierrors.IsNotFound(envTestClient.Get(context.Background(), client.ObjectKeyFromObject(dsn), dsn))).To(BeTrue())
	g.Expect(apierrors.IsNotFound(envTestClient.Get(context.Background(), client.ObjectKeyFromObject(migrate), migrate))).To(BeTrue())

	g.Expect(envTestClient.Get(context.Background(), client.ObjectKeyFromObject(app), app)).To(Succeed())
	g.Expect(envTestClient.Get(context.Background(), client.ObjectKeyFromObject(post), post)).To(Succeed())

	saUID := sa.UID
	postUID := post.UID

	// Second apply recreates SA/post (new UIDs) and still leaves hooks deleted.
	_, err = executeCommand(applyCmd)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(envTestClient.Get(context.Background(), client.ObjectKeyFromObject(sa), sa)).To(Succeed())
	g.Expect(sa.UID).ToNot(Equal(saUID))

	g.Expect(envTestClient.Get(context.Background(), client.ObjectKeyFromObject(post), post)).To(Succeed())
	g.Expect(post.UID).ToNot(Equal(postUID))

	g.Expect(apierrors.IsNotFound(envTestClient.Get(context.Background(), client.ObjectKeyFromObject(dsn), dsn))).To(BeTrue())
	g.Expect(apierrors.IsNotFound(envTestClient.Get(context.Background(), client.ObjectKeyFromObject(migrate), migrate))).To(BeTrue())

	// Final inventory must not retain delete-after-hooks objects.
	sm, err := runtime.NewResourceManager(kubeconfigArgs)
	g.Expect(err).ToNot(HaveOccurred())
	storage := runtime.NewStorageManager(sm)
	inst, err := storage.Get(context.Background(), name, namespace)
	g.Expect(err).ToNot(HaveOccurred())
	for _, entry := range inst.Inventory.Entries {
		g.Expect(entry.ID).ToNot(ContainSubstring(name + "-dsn"))
		g.Expect(entry.ID).ToNot(ContainSubstring(name + "-migrate"))
	}
}
