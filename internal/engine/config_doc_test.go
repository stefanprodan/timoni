/*
Copyright 2024 Stefan Prodan
SPDX-License-Identifier: Apache-2.0

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
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	. "github.com/onsi/gomega"
)

func TestConfigDoc_typeExpr(t *testing.T) {
	tests := []struct {
		expr string
		want string
	}{
		{`*512 | int & >=64`, `int & >=64`},
		{`*"soft" | "hard" | "none"`, `"hard" | "none"`},
		{`*{"kubernetes.io/os": "linux"} | {[string]: string}`, `{[string]: string}`},
		{`timoniv1.#Metadata & {#Version: moduleVersion}`, `timoniv1.#Metadata`},
		{`corev1.#ResourceRequirements & {requests: cpu: "100m"}`, `corev1.#ResourceRequirements`},
		{`[...corev1.#Toleration]`, `[...corev1.#Toleration]`},
		{`string & =~"^[a-z]+$"`, `string & =~"^[a-z]+$"`},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			g := NewWithT(t)
			e, err := parser.ParseExpr("test.cue", tt.expr)
			g.Expect(err).ToNot(HaveOccurred())

			got := stripStructs(stripDefaults(e))
			g.Expect(got).ToNot(BeNil())
			b, err := format.Node(got, format.Simplify())
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(strings.Join(strings.Fields(string(b)), " ")).To(Equal(tt.want))
		})
	}
}

func TestConfigDoc_stripAll(t *testing.T) {
	g := NewWithT(t)
	e, err := parser.ParseExpr("test.cue", `{enabled: *true | bool}`)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(stripStructs(e)).To(BeNil())
}

func TestConfigDoc_isLiteral(t *testing.T) {
	g := NewWithT(t)
	g.Expect(isLiteral(`"test"`)).To(BeTrue())
	g.Expect(isLiteral(`10`)).To(BeTrue())
	g.Expect(isLiteral(`true`)).To(BeTrue())
	g.Expect(isLiteral(`string`)).To(BeFalse())
	g.Expect(isLiteral(`timoniv1.#Image`)).To(BeFalse())
}

func TestConfigDoc_FormatConfigCUE(t *testing.T) {
	g := NewWithT(t)
	modPath, err := filepath.Abs("../../cmd/timoni/testdata/module")
	g.Expect(err).ToNot(HaveOccurred())

	b := NewModuleBuilder(cuecontext.New(), "test", "default", modPath, defaultPackage)
	g.Expect(b.OverlaySchemaFile()).To(Succeed())
	v, err := b.Build()
	g.Expect(err).ToNot(HaveOccurred())

	fields, err := b.GetConfigDoc(v)
	g.Expect(err).ToNot(HaveOccurred())

	out, err := FormatConfigCUE(fields)
	g.Expect(err).ToNot(HaveOccurred())

	_, err = parser.ParseFile("config.cue", out)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(out).To(ContainSubstring("metadata: timoniv1.#Metadata"))
	g.Expect(out).To(ContainSubstring("// Log level, info by default\n\tlogLevel?: *\"info\" | \"debug\" | \"info\"\n"))
	g.Expect(out).To(MatchRegexp(`priority: +\*1 | int & >=0\n`))
	g.Expect(out).To(ContainSubstring("team: \"test\"\n"))
	g.Expect(out).To(ContainSubstring("\"app.kubernetes.io/team\": \"test\"\n"))
	g.Expect(out).To(ContainSubstring("podAnnotations?: {[string]: string}\n"))
	g.Expect(out).ToNot(ContainSubstring("kubeVersion!"))

	keys := map[string]ConfigField{}
	for _, f := range fields {
		keys[f.Key()] = f
	}
	g.Expect(keys).To(HaveKey("metadata: labels: \"app.kubernetes.io/team\":"))
	g.Expect(keys["logLevel:"].Optional).To(BeTrue())
	g.Expect(keys["logLevel:"].Default).To(Equal("\"info\""))
	g.Expect(keys["client:"].NoDoc).To(BeTrue())
}
