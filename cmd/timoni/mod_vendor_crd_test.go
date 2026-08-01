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
	"fmt"
	"os"
	"os/exec"
	"path"
	"testing"

	"github.com/mattn/go-shellwords"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRemoveCRDStatusSchema(t *testing.T) {
	tests := []struct {
		name     string
		versions interface{}
		wantErr  string
	}{
		{
			name:     "non-object version",
			versions: []interface{}{"v1"},
			wantErr:  "spec.versions[0] must be an object",
		},
		{
			name:     "non-list versions",
			versions: "v1",
			wantErr:  "reading CRD spec.versions failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			crd := &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{"versions": tt.versions},
			}}

			err := removeCRDStatusSchema(crd)

			g.Expect(err).To(MatchError(ContainSubstring(tt.wantErr)))
		})
	}
}

func TestRemoveCRDStatusSchemaPreservesOtherFields(t *testing.T) {
	g := NewWithT(t)
	crd := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"versions": []interface{}{
				map[string]interface{}{
					"name": "v1",
					"schema": map[string]interface{}{
						"openAPIV3Schema": map[string]interface{}{
							"properties": map[string]interface{}{
								"spec":   map[string]interface{}{"type": "object"},
								"status": map[string]interface{}{"type": "object"},
							},
						},
					},
				},
			},
		},
	}}

	err := removeCRDStatusSchema(crd)

	g.Expect(err).ToNot(HaveOccurred())
	versions, found, err := unstructured.NestedSlice(crd.Object, "spec", "versions")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	version := versions[0].(map[string]interface{})
	properties, found, err := unstructured.NestedMap(version, "schema", "openAPIV3Schema", "properties")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(properties).To(HaveKey("spec"))
	g.Expect(properties).ToNot(HaveKey("status"))
}

func TestVendorCrd(t *testing.T) {
	// To regenerate the golden files:
	// make install
	// cd cmd/timoni/
	// timoni mod vendor crd testdata/crd/golden/ -f testdata/crd/source/cert-manager.crds.yaml
	// timoni mod vendor crd testdata/crd/golden/ -f testdata/crd/source/flagger.crds.yaml
	goldenPath := "testdata/crd/golden/cue.mod/"

	tmpDir := t.TempDir()
	genPath := path.Join(tmpDir, "cue.mod")

	g := NewWithT(t)

	err := os.MkdirAll(genPath, os.ModePerm)
	g.Expect(err).ToNot(HaveOccurred())

	for crdPath, outputMatcher := range map[string]types.GomegaMatcher{
		"testdata/crd/source/cert-manager.crds.yaml": ContainSubstring("cert-manager.io/issuer/v1"),
		"testdata/crd/source/flagger.crds.yaml":      ContainSubstring("flagger.app/canary/v1beta1"),
	} {
		output, err := executeCommand(fmt.Sprintf(
			"mod vendor crd %s -f %s",
			tmpDir,
			crdPath,
		))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(outputMatcher)
	}

	diffArgs := fmt.Sprintf("--no-pager diff --no-index %s %s", genPath, goldenPath)

	args, err := shellwords.Parse(diffArgs)
	g.Expect(err).ToNot(HaveOccurred())

	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	g.Expect(string(out)).To(BeEmpty())
	g.Expect(err).ToNot(HaveOccurred())
}
