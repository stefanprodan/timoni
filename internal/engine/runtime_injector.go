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
	"fmt"
	"strconv"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/cue/token"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// RuntimeInjector injects field values in CUE files based on @timoni() attributes.
type RuntimeInjector struct {
	ctx *cue.Context
}

// NewRuntimeInjector creates an RuntimeInjector for the given context.
func NewRuntimeInjector(ctx *cue.Context) *RuntimeInjector {
	return &RuntimeInjector{ctx: ctx}
}

// Inject searches for Timoni's attributes and
// sets the CUE field value to the runtime value.
// If an attribute does not match any runtime value,
// the CUE field is left untouched.
func (in *RuntimeInjector) Inject(node ast.Node, vars map[string]string) ([]byte, error) {
	output, err := in.inject(node, vars)
	if err != nil {
		return nil, err
	}

	data, err := format.Node(output)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (in *RuntimeInjector) ListAttributes(f *ast.File) map[string]string {
	attrs := make(map[string]string)

	ast.Walk(f, nil, func(n ast.Node) {
		switch x := n.(type) {
		case *ast.Field:
			for _, a := range x.Attrs {
				if apiv1.IsRuntimeAttribute(a.Split()) {
					ra, _ := apiv1.NewRuntimeAttribute(a.Split())
					attrs[ra.Name] = ra.Type
				}
			}
		}
	})

	return attrs
}

func (in *RuntimeInjector) inject(node ast.Node, vars map[string]string) (ast.Node, error) {
	var err error
	f := func(c astutil.Cursor) bool {
		n := c.Node()
		switch n.(type) {
		case *ast.Field:
			field := n.(*ast.Field)
			if len(field.Attrs) == 0 {
				return true
			}

			var key, body string
			for _, a := range field.Attrs {
				key, body = a.Split()
				if key == apiv1.FieldManager {
					break
				}
			}

			if !apiv1.IsRuntimeAttribute(key, body) {
				return true
			}

			ra, _ := apiv1.NewRuntimeAttribute(key, body)

			if envVal, ok := vars[ra.Name]; ok {
				switch ra.Type {
				case "string":
					field.Value = ast.NewLit(token.STRING, in.quoteString(envVal))
				case "number":
					field.Value = ast.NewLit(token.INT, envVal)
				case "bool":
					field.Value = ast.NewIdent(envVal)
				default:
					err = fmt.Errorf("failed to parse attribute '@%s(%s)', unknown type '%s' must be string, number or bool",
						apiv1.FieldManager, body, ra.Type)
					return false
				}
				c.Replace(field)
			}
		}
		return true
	}

	return astutil.Apply(node, f, nil), err
}

func (in *RuntimeInjector) quoteString(s string) string {
	lines := []string{}
	last := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[last:i])
			last = i + 1
		}
		if c == '\r' {
			goto quoted
		}
	}
	lines = append(lines, s[last:])
	if len(lines) >= 2 {
		buf := []byte{}
		buf = append(buf, `"""`+"\n"...)
		for _, l := range lines {
			if l == "" {
				// no indentation for empty lines
				buf = append(buf, '\n')
				continue
			}
			buf = append(buf, '\t')
			p := len(buf)
			buf = strconv.AppendQuote(buf, l)
			// remove quotes
			buf[p] = '\t'
			buf[len(buf)-1] = '\n'
		}
		buf = append(buf, "\t\t"+`"""`...)
		return string(buf)
	}
quoted:
	return literal.String.Quote(s)
}
