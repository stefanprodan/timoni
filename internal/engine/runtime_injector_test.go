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

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	. "github.com/onsi/gomega"
)

func TestInjector_Env(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	t.Setenv("USERNAME", "stefanprodan")
	key := `-----BEGIN PGP PUBLIC KEY BLOCK-----

mQSuBF9+HgMRDADKT8UBcSzpTi4JXt/ohhVW3x81AGFPrQvs6MYrcnNJfIkPTJD8
.........
=/4e+
-----END PGP PUBLIC KEY BLOCK-----`
	t.Setenv("PGP_PUB_KEY", key)
	t.Setenv("AGE", "41")
	t.Setenv("IS_ADMIN", "true")

	input := `package test

// these secret values are injected at apply time from OS ENV
secrets: {
	username: *"test" | string @timoni(runtime:string:USERNAME)

	// The OpenPGP key will be injected as a multi-line string
	key: string @timoni(runtime:string:PGP_PUB_KEY)

	age:     int  @timoni(runtime:number:AGE)
	isAdmin: bool @timoni(runtime:bool:IS_ADMIN)
}
`
	output := `package test

// these secret values are injected at apply time from OS ENV
secrets: {
	username: "stefanprodan" @timoni(runtime:string:USERNAME)

	// The OpenPGP key will be injected as a multi-line string
	key: """
		-----BEGIN PGP PUBLIC KEY BLOCK-----

		mQSuBF9+HgMRDADKT8UBcSzpTi4JXt/ohhVW3x81AGFPrQvs6MYrcnNJfIkPTJD8
		.........
		=/4e+
		-----END PGP PUBLIC KEY BLOCK-----
		""" @timoni(runtime:string:PGP_PUB_KEY)

	age:     41   @timoni(runtime:number:AGE)
	isAdmin: true @timoni(runtime:bool:IS_ADMIN)
}
`

	f, err := parser.ParseFile("", []byte(input), parser.ParseComments)
	g.Expect(err).ToNot(HaveOccurred())

	vb := NewRuntimeInjector(ctx)

	attrs := vb.ListAttributes(f)
	g.Expect(attrs).To(BeEquivalentTo(map[string]string{
		"PGP_PUB_KEY": "string",
		"AGE":         "number",
		"IS_ADMIN":    "bool",
		"USERNAME":    "string",
	}))

	result, err := vb.Inject(f, GetEnv())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(result)).To(BeIdenticalTo(output))
}

func TestInjector_InvalidValues(t *testing.T) {
	input := `package test

secrets: {
	age:     int  @timoni(runtime:number:AGE)
	isAdmin: bool @timoni(runtime:bool:IS_ADMIN)
}
`

	tests := []struct {
		name string
		vars map[string]string
		err  string
	}{
		{
			name: "number with newline",
			vars: map[string]string{"AGE": "41\nfoo: 1"},
			err:  "value must be a number",
		},
		{
			name: "number with braces",
			vars: map[string]string{"AGE": "41}\nfoo: {"},
			err:  "value must be a number",
		},
		{
			name: "number with expression",
			vars: map[string]string{"AGE": "41 + 1"},
			err:  "value must be a number",
		},
		{
			name: "number with comment",
			vars: map[string]string{"AGE": "41 // comment"},
			err:  "value must be a number",
		},
		{
			name: "number empty",
			vars: map[string]string{"AGE": ""},
			err:  "value must be a number",
		},
		{
			name: "bool with newline",
			vars: map[string]string{"IS_ADMIN": "true\nfoo: 1"},
			err:  "value must be 'true' or 'false'",
		},
		{
			name: "bool with braces",
			vars: map[string]string{"IS_ADMIN": "true}\nfoo: {"},
			err:  "value must be 'true' or 'false'",
		},
		{
			name: "bool with reference",
			vars: map[string]string{"IS_ADMIN": "someIdent"},
			err:  "value must be 'true' or 'false'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := cuecontext.New()

			f, err := parser.ParseFile("", []byte(input), parser.ParseComments)
			g.Expect(err).ToNot(HaveOccurred())

			result, err := NewRuntimeInjector(ctx).Inject(f, tt.vars)
			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring(tt.err))
			g.Expect(err.Error()).To(ContainSubstring("failed to parse attribute '@timoni("))
			g.Expect(result).To(BeNil())
		})
	}
}

func TestInjector_NumberFormats(t *testing.T) {
	input := `package test

values: n: number @timoni(runtime:number:N)
`

	tests := []struct {
		value string
		want  string
	}{
		{value: "41", want: "41"},
		{value: "-41", want: "-41"},
		{value: "4.1", want: "4.1"},
		{value: "-4.1e-2", want: "-4.1e-2"},
		{value: "0x1f", want: "0x1f"},
		{value: "1_000", want: "1_000"},
		{value: "2Mi", want: "2Mi"},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			g := NewWithT(t)
			ctx := cuecontext.New()

			f, err := parser.ParseFile("", []byte(input), parser.ParseComments)
			g.Expect(err).ToNot(HaveOccurred())

			result, err := NewRuntimeInjector(ctx).Inject(f, map[string]string{"N": tt.value})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(string(result)).To(ContainSubstring("n: " + tt.want + " @timoni("))

			// The injected value must round-trip as a CUE number.
			v := ctx.CompileBytes(result).LookupPath(cue.ParsePath("values.n"))
			g.Expect(v.Err()).ToNot(HaveOccurred())
			g.Expect(v.Kind()).To(BeElementOf(cue.IntKind, cue.FloatKind))
		})
	}
}

func TestInjector_Operand(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	t.Setenv("USERNAME", "stefanprodan")
	t.Setenv("AGE", "41")
	t.Setenv("IS_ADMIN", "true")

	input := `package main

secrets: {
	username?: string @timoni(runtime:string:USERNAME)

	age:     int  @timoni(runtime:number:AGE)
	isAdmin: bool @timoni(runtime:bool:IS_ADMIN)
}
`
	output := `package main

secrets: {
	username?: "stefanprodan" @timoni(runtime:string:USERNAME)

	age:     41   @timoni(runtime:number:AGE)
	isAdmin: true @timoni(runtime:bool:IS_ADMIN)
}
`

	f, err := parser.ParseFile("", []byte(input), parser.ParseComments)
	g.Expect(err).ToNot(HaveOccurred())

	vb := NewRuntimeInjector(ctx)

	result, err := vb.Inject(f, GetEnv())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(result)).To(BeIdenticalTo(output))
}
