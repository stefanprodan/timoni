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

package flags

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestValidateModuleVersion(t *testing.T) {
	g := NewWithT(t)

	for _, version := range []string{"1.0.0", "2.0.0-rc.1"} {
		g.Expect(ValidateModuleVersion(version)).To(Succeed())
	}

	for version, expected := range map[string]string{
		"":           "version is required",
		"1.0":        "version is not in semver format",
		"1.0.0+demo": "version build metadata is not supported",
	} {
		g.Expect(ValidateModuleVersion(version)).To(MatchError(ContainSubstring(expected)))
	}
}
