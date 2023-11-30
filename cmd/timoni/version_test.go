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
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestVersion(t *testing.T) {
	g := NewWithT(t)
	output, err := executeCommand("version -o yaml")
	g.Expect(err).ToNot(HaveOccurred())

	var data map[string]interface{}
	err = yaml.Unmarshal([]byte(output), &data)
	g.Expect(err).ToNot(HaveOccurred())

	expectedAPIVersion := apiv1.GroupVersion.String()
	g.Expect(data).To(HaveKeyWithValue("api", expectedAPIVersion))
	g.Expect(data).To(HaveKey("client"))
	g.Expect(data).To(HaveKey("cue"))
}
