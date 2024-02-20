package testutils

import (
	"testing"

	. "github.com/onsi/gomega"
)

type testStruct struct{}

func (t *testStruct) TestMethod() {}

func TestImplements(t *testing.T) {
	g := NewWithT(t)

	type truthteller interface {
		TestMethod()
	}

	type liar interface {
		NotTestMethod()
	}

	g.Expect(Implements[truthteller](&testStruct{})).To(BeTrueBecause("testStruct implements truthteller"))
	g.Expect(Implements[liar](&testStruct{})).To(BeFalseBecause("testStruct does not implement liar"))
}
