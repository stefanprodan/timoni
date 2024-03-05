/*
Copyright 2024 Stefan Prodan

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
