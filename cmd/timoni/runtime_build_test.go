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
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_RuntimeBuild(t *testing.T) {
	g := NewWithT(t)

	resName := rnd("my-data", 5)

	runtimeData := fmt.Sprintf(`
runtime: {
	apiVersion: "v1alpha1"
	name:       "test"
	values: [
		{
			query: "k8s:v1:Secret:%[1]s:%[2]s"
			for: {
				"DOMAIN":   "obj.data.domain"
				"ENABLED":  "obj.data.enabled"
			}
		},
		{
			query: "k8s:v1:ConfigMap:%[1]s:%[2]s"
			for: {
				"DOMAIN":   "obj.data.domain"
				"ENABLED":  "obj.data.enabled"
			}
			optional: true
		}
	]
}
`, "default", resName)

	runtimePath := filepath.Join(t.TempDir(), "runtime.cue")
	err := os.WriteFile(runtimePath, []byte(runtimeData), 0644)
	g.Expect(err).ToNot(HaveOccurred())

	sc := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resName,
			Namespace: "default",
		},
		StringData: map[string]string{
			"domain":  "sc.local",
			"enabled": "false",
		},
	}

	err = envTestClient.Create(context.Background(), sc, &client.CreateOptions{
		FieldManager: "timoni",
	})
	g.Expect(err).ToNot(HaveOccurred())

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resName,
			Namespace: "default",
		},
		Data: map[string]string{
			"domain":  "cm.local",
			"enabled": "true",
		},
	}

	err = envTestClient.Create(context.Background(), cm, &client.CreateOptions{
		FieldManager: "timoni",
	})
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("builds runtime from ConfigMap", func(t *testing.T) {
		g := NewWithT(t)

		cmd := fmt.Sprintf("runtime build -f=%s",
			runtimePath,
		)

		output, err := executeCommand(cmd)
		g.Expect(err).ToNot(HaveOccurred())
		t.Log("\n", output)
		g.Expect(output).To(ContainSubstring("cm.local"))
	})

	t.Run("builds runtime from Secret", func(t *testing.T) {
		g := NewWithT(t)

		err := envTestClient.Delete(context.Background(), cm)
		g.Expect(err).ToNot(HaveOccurred())

		cmd := fmt.Sprintf("runtime build -f=%s",
			runtimePath,
		)

		output, err := executeCommand(cmd)
		g.Expect(err).ToNot(HaveOccurred())
		t.Log("\n", output)
		g.Expect(output).To(ContainSubstring("sc.local"))
	})
}
