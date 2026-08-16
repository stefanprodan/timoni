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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/stefanprodan/timoni/internal/fscopy"
)

// newTestModule copies the testdata module to a temporary directory
// and writes the given CUE test cases to its templates package.
func newTestModule(t *testing.T, cases string) string {
	t.Helper()
	g := NewWithT(t)

	modPath := filepath.Join(t.TempDir(), "module")
	// The module vendors the core schemas as relative symlinks under cue.mod/pkg,
	// which must be materialized for the copy to build on its own.
	err := fscopy.CopyDir("testdata/module", modPath, fscopy.Options{FollowSymlinks: true})
	g.Expect(err).ToNot(HaveOccurred())

	err = os.WriteFile(filepath.Join(modPath, "templates", "cases_test.cue"), []byte(cases), 0644)
	g.Expect(err).ToNot(HaveOccurred())

	return modPath
}

func Test_ModTest(t *testing.T) {
	g := NewWithT(t)

	modPath := newTestModule(t, `package templates

cases: "client config points at the default domain": {
	objects: "ConfigMap/test/test-client": data: server: "tcp://example.internal:9090"
}

cases: "domain is configurable": {
	values: domain: "example.com"
	objects: "ConfigMap/test/test-client": data: server: "tcp://example.com:9090"
}

cases: "server config is rendered alongside the client": {
	objects: {...}
	assert: "both config maps are rendered":
		objects["ConfigMap/test/test-client"].kind == objects["ConfigMap/test/test-server"].kind
}
`)

	output, err := executeCommand(fmt.Sprintf("mod test %s", modPath))
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(output).To(ContainSubstring("client config points at the default domain"))
	g.Expect(output).To(ContainSubstring("domain is configurable"))
	g.Expect(output).To(ContainSubstring("3 test cases passed"))
}

func Test_ModTestFailures(t *testing.T) {
	tests := []struct {
		name     string
		cases    string
		contains string
	}{
		{
			name: "conflicting object field",
			cases: `cases: "wrong domain": {
	objects: "ConfigMap/test/test-client": data: server: "tcp://wrong:9090"
}`,
			contains: "conflicting values",
		},
		{
			name: "object the module does not render",
			cases: `cases: "missing object": {
	objects: "Deployment/test/test": spec: replicas: 1
}`,
			contains: "the module renders",
		},
		{
			name: "field the module does not render",
			cases: `cases: "misspelled field": {
	objects: "ConfigMap/test/test-client": data: sever: "tcp://example.internal:9090"
}`,
			contains: `expected field(s) "ConfigMap/test/test-client".data.sever not rendered`,
		},
		{
			name: "unknown case field",
			cases: `cases: "misspelled case field": {
	kubeVerison: "1.23.0"
	objects: "ConfigMap/test/test-client": kind: "ConfigMap"
}`,
			contains: "unknown field(s) kubeVerison",
		},
		{
			name: "case field that is not a string",
			cases: `cases: "kube version as a number": {
	kubeVersion: 123
	objects: "ConfigMap/test/test-client": kind: "ConfigMap"
}`,
			contains: "kubeVersion: cannot use value 123 (type int) as string",
		},
		{
			name: "assertion that does not hold",
			cases: `cases: "false assertion": {
	objects: {...}
	assert: "client is a Deployment": objects["ConfigMap/test/test-client"].kind == "Deployment"
}`,
			contains: `assertion "client is a Deployment" does not hold`,
		},
		{
			name: "values rejected by the config schema",
			cases: `cases: "invalid values": {
	values: domain: 42
}`,
			contains: `values.domain: conflicting values 42 and "example.internal"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			modPath := newTestModule(t, "package templates\n\n"+tt.cases+"\n")

			output, err := executeCommand(fmt.Sprintf("mod test %s", modPath))
			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring("test cases failed"))
			g.Expect(output).To(ContainSubstring(tt.contains))
		})
	}
}

// Test_ModTestDefaultsAreCompared guards against an expectation being absorbed
// by a disjunction instead of compared against the rendered value. The module's
// port is declared as a default, so unifying an expectation with the unresolved
// template would hold rather than conflict.
func Test_ModTestDefaultsAreCompared(t *testing.T) {
	g := NewWithT(t)

	modPath := newTestModule(t, `package templates

cases: "a wrong port must not be absorbed by the default": {
	objects: "ConfigMap/test/test-client": data: server: "tcp://example.internal:1234"
}
`)

	output, err := executeCommand(fmt.Sprintf("mod test %s", modPath))
	g.Expect(err).To(HaveOccurred())
	g.Expect(output).To(ContainSubstring("conflicting values"))
}

func Test_ModTestRunFilter(t *testing.T) {
	g := NewWithT(t)

	modPath := newTestModule(t, `package templates

cases: "alpha case": {
	objects: "ConfigMap/test/test-client": kind: "ConfigMap"
}

cases: "beta case": {
	objects: "ConfigMap/test/test-server": kind: "ConfigMap"
}
`)

	output, err := executeCommand(fmt.Sprintf("mod test %s --run ^alpha", modPath))
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(output).To(ContainSubstring("alpha case"))
	g.Expect(output).ToNot(ContainSubstring("beta case"))
	g.Expect(output).To(ContainSubstring("1 test cases passed"))
}

// Test_ModTestReportsEveryConflict checks that a case with several failing
// expectations reports all of them, so that a fix does not have to be found
// one run at a time.
func Test_ModTestReportsEveryConflict(t *testing.T) {
	g := NewWithT(t)

	modPath := newTestModule(t, `package templates

cases: "every field is wrong": {
	objects: "ConfigMap/test/test-client": {
		kind: "Secret"
		data: server: "tcp://wrong:9090"
	}
}
`)

	output, err := executeCommand(fmt.Sprintf("mod test %s", modPath))
	g.Expect(err).To(HaveOccurred())

	g.Expect(output).To(ContainSubstring(`.kind: conflicting values "ConfigMap" and "Secret"`))
	g.Expect(output).To(ContainSubstring(`.data.server: conflicting values`))
	// The position of each conflict is reported relative to the module root.
	g.Expect(output).To(ContainSubstring("./templates/cases_test.cue:"))
}

func Test_ModTestRunFilterNoMatch(t *testing.T) {
	g := NewWithT(t)

	modPath := newTestModule(t, `package templates

cases: "alpha case": {
	objects: "ConfigMap/test/test-client": kind: "ConfigMap"
}
`)

	output, err := executeCommand(fmt.Sprintf("mod test %s --run ^gamma", modPath))
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring(`no test cases match the --run expression "^gamma"`))
	g.Expect(output).ToNot(ContainSubstring("no test cases found"))
}

func Test_ModTestNoCases(t *testing.T) {
	g := NewWithT(t)

	output, err := executeCommand("mod test testdata/module")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(ContainSubstring("no test cases found"))
}
