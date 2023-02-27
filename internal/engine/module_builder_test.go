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
	"fmt"
	"path"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/gomega"
)

func TestModuleBuilder(t *testing.T) {
	g := NewWithT(t)
	moduleRoot := path.Join(t.TempDir(), "module")

	err := CopyModule("testdata/module", moduleRoot)
	g.Expect(err).ToNot(HaveOccurred())

	ctx := cuecontext.New()

	mb := NewModuleBuilder(ctx, "test-name", "test-namespace", moduleRoot, "main")

	moduleName, err := mb.GetModuleName()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(moduleName).To(BeEquivalentTo("timoni.sh/test"))

	err = mb.MergeValuesFile([]string{"testdata/module-values/overlay.cue"})
	g.Expect(err).ToNot(HaveOccurred())

	val, err := mb.Build()
	g.Expect(err).ToNot(HaveOccurred())

	objects := val.LookupPath(cue.ParsePath("timoni.apply.all"))
	g.Expect(objects.Err()).ToNot(HaveOccurred())

	gold, err := ExtractValueFromFile(ctx, "testdata/module-golden/overlay.cue", "objects")
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(fmt.Sprintf("%v", objects)).To(BeEquivalentTo(fmt.Sprintf("%v", gold)))
}

func TestExtractBuildResult(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	steps, err := ExtractValueFromFile(ctx, "testdata/api/apply-steps.cue", defaultOutputExp)
	g.Expect(err).ToNot(HaveOccurred())

	sets, err := GetResources(steps)
	g.Expect(err).ToNot(HaveOccurred())

	expectedNames := []string{"app", "addons", "tests"}
	for s, set := range sets {
		g.Expect(sets[s].Name).To(BeEquivalentTo(expectedNames[s]))
		g.Expect(len(set.Objects)).To(BeEquivalentTo(2))
	}
}
