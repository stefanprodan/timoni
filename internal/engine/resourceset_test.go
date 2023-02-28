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

package engine

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/gomega"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestGetResources(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	steps, err := ExtractValueFromFile(ctx, "testdata/api/apply-steps.cue", apiv1.ApplySelector.String())
	g.Expect(err).ToNot(HaveOccurred())

	sets, err := GetResources(steps)
	g.Expect(err).ToNot(HaveOccurred())

	expectedNames := []string{"app", "addons", "tests"}
	for s, set := range sets {
		g.Expect(sets[s].Name).To(BeEquivalentTo(expectedNames[s]))
		g.Expect(len(set.Objects)).To(BeEquivalentTo(2))
	}
}
