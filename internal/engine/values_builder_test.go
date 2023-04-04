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
	"os"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/gomega"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func mustReadFile(g Gomega, path string) []byte {
	bytes, err := os.ReadFile(path)
	g.ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return bytes
}

func TestValuesBuilder(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	vb := NewValuesBuilder(ctx)

	base := "testdata/values/base.cue"
	overlays := [][]byte{
		mustReadFile(g, "testdata/values/overlay-1.cue"),
		mustReadFile(g, "testdata/values/overlay-2.cue"),
	}

	finalVal, err := vb.MergeValues(overlays, base)
	g.Expect(err).ToNot(HaveOccurred())

	goldVal, err := ExtractValueFromFile(ctx, "testdata/values/golden.cue", apiv1.ValuesSelector.String())
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(fmt.Sprintf("%v", finalVal)).To(BeEquivalentTo(fmt.Sprintf("%v", goldVal)))
}
