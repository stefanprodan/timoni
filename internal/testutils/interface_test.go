package testutils

import (
	"testing"
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

	a := &testStruct{}

	g.Expect(a).To(Implement((*truthteller)(nil)))
	g.Expect(a).ToNot(Implement((*liar)(nil)))
}
