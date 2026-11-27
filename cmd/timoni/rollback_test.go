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
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/stefanprodan/timoni/internal/runtime"
)

func TestRollback_PreviousRevision(t *testing.T) {
	g := NewWithT(t)

	modPath := "testdata/module"
	namespace := rnd("rb-ns")
	name := rnd("rb-app")
	modName := rnd("rb-mod")
	modURL := fmt.Sprintf("%s/%s", dockerRegistry, modName)

	push := func(ver string) string {
		t.Helper()
		out, err := executeCommand(fmt.Sprintf(
			"mod push %s oci://%s -v %s -o json --resolve-symlinks",
			modPath, modURL, ver,
		))
		g.Expect(err).ToNot(HaveOccurred())
		var mod struct {
			Digest string `json:"digest"`
		}
		g.Expect(json.Unmarshal([]byte(out), &mod)).To(Succeed())
		g.Expect(mod.Digest).ToNot(BeEmpty())
		return mod.Digest
	}

	digestV1 := push("1.0.0")
	digestV2 := push("2.0.0")
	g.Expect(digestV1).ToNot(Equal(digestV2))

	_, err := executeCommand(fmt.Sprintf(
		"apply -n %s %s oci://%s -v 1.0.0 -d %s -p main --wait",
		namespace, name, modURL, digestV1,
	))
	g.Expect(err).ToNot(HaveOccurred())

	_, err = executeCommand(fmt.Sprintf(
		"apply -n %s %s oci://%s -v 2.0.0 -d %s -p main --wait",
		namespace, name, modURL, digestV2,
	))
	g.Expect(err).ToNot(HaveOccurred())

	sm, err := runtime.NewResourceManager(kubeconfigArgs)
	g.Expect(err).ToNot(HaveOccurred())
	storage := runtime.NewStorageManager(sm)

	inst, err := storage.Get(context.Background(), name, namespace)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(inst.Module.Digest).To(Equal(digestV2))

	prev, err := storage.GetPrevious(context.Background(), name, namespace)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(prev).ToNot(BeNil())
	g.Expect(prev.Module.Digest).To(Equal(digestV1))

	_, err = executeCommand(fmt.Sprintf("rollback -n %s %s --wait", namespace, name))
	g.Expect(err).ToNot(HaveOccurred())

	inst, err = storage.Get(context.Background(), name, namespace)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(inst.Module.Digest).To(Equal(digestV1))

	prev, err = storage.GetPrevious(context.Background(), name, namespace)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(prev).To(BeNil())

	_, err = executeCommand(fmt.Sprintf("rollback -n %s %s --wait", namespace, name))
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("no previous revision"))
}
