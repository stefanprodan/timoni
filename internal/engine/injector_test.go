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

	vb := NewInjector(ctx)

	base := "testdata/inject/env.cue"

	result, err := vb.Inject(base)
	g.Expect(err).ToNot(HaveOccurred())

	want := `package test

// these secret values are injected at apply time from OS ENV
secrets: {
	username: "stefanprodan" @timoni(env:string:USERNAME)

	// The OpenPGP key will be injected as a multi-line string
	key: """
		-----BEGIN PGP PUBLIC KEY BLOCK-----

		mQSuBF9+HgMRDADKT8UBcSzpTi4JXt/ohhVW3x81AGFPrQvs6MYrcnNJfIkPTJD8
		.........
		=/4e+
		-----END PGP PUBLIC KEY BLOCK-----
		""" @timoni(env:string:PGP_PUB_KEY)

	age:     41   @timoni(env:number:AGE)
	isAdmin: true @timoni(env:bool:IS_ADMIN)
}
`
	g.Expect(string(result)).To(BeIdenticalTo(want))
}
