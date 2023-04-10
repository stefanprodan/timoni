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
	"testing"

	. "github.com/onsi/gomega"

	"github.com/stefanprodan/timoni/internal/engine"
)

func Test_ListMod(t *testing.T) {
	g := NewWithT(t)
	modPath := "testdata/module"
	modURL := fmt.Sprintf("%s/%s", dockerRegistry, rnd("my-mod", 5))
	modVers := []string{"1.0.0", "2.0.0", "1.1.0-rc.1"}

	for _, v := range modVers {
		_, err := executeCommand(fmt.Sprintf(
			"mod push %s oci://%s -v %s --latest",
			modPath,
			modURL,
			v,
		))
		g.Expect(err).ToNot(HaveOccurred())
	}

	output, err := executeCommand(fmt.Sprintf(
		"mod ls oci://%s",
		modURL,
	))
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(output).To(ContainSubstring(engine.LatestTag))
	for _, v := range modVers {
		g.Expect(output).To(ContainSubstring(v))
	}
}
