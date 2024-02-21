package testutils

import (
	"fmt"
	"reflect"

	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
)

// Implements checks if the object implements the interface
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
