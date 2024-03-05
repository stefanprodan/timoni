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
	"fmt"
	"reflect"

	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
)

// Implement checks if the object implements the interface
func Implement(expected interface{}) types.GomegaMatcher {
	return &implementsMatcher{
		expected: expected,
	}
}

type implementsMatcher struct {
	expected interface{}
}

func (m *implementsMatcher) Match(actual interface{}) (success bool, err error) {
	iface := reflect.TypeOf(m.expected).Elem()
	t := reflect.TypeOf(actual)

	return t.Implements(iface), nil
}

func (m *implementsMatcher) FailureMessage(actual interface{}) (message string) {
	return format.Message(actual, fmt.Sprintf("to implement %T", m.expected))
}

func (m *implementsMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return format.Message(actual, fmt.Sprintf("not to implement %T", m.expected))
}
