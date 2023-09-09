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
