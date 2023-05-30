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
	"os"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// Injector injects field values in CUE files based on @timoni() attributes.
type Injector struct {
	ctx *cue.Context
}

// NewInjector creates an Injector for the given context.
func NewInjector(ctx *cue.Context) *Injector {
	return &Injector{ctx: ctx}
}

// Inject searches for attributes in the format
// '@timoni(env:[string|number|bool]:[ENV_VAR_NAME])'
// and sets the CUE field value to the env var value.
// If an env var is not found in the current environment,
// the CUE field is left untouched.
func (in *Injector) Inject(src string) ([]byte, error) {
	tree, err := parser.ParseFile(src, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	output, err := in.injectFromEnv(tree)
	if err != nil {
		return nil, err
	}

	data, err := format.Node(output)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (in *Injector) injectFromEnv(tree *ast.File) (ast.Node, error) {
	var re error
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

			if key == "" {
				return true
			}

			parts := strings.Split(body, ":")

			if len(parts) != 3 || parts[0] != "env" {
				return true
			}

			envKind := parts[1]
			envKey := parts[2]

			if envVal, ok := os.LookupEnv(envKey); ok {
				switch envKind {
				case "string":
					field.Value = ast.NewLit(token.STRING, in.quoteString(envVal))
				case "number":
					field.Value = ast.NewLit(token.INT, envVal)
				case "bool":
					field.Value = ast.NewIdent(envVal)
				default:
					re = fmt.Errorf("failed to parse attribute '@%s(%s)', unknown type '%s' must be string, number or bool",
						apiv1.FieldManager, body, envKind)
					return false
				}
				c.Replace(field)
			}
		}
		return true
	}

	return astutil.Apply(tree, f, nil), re
}

func (in *Injector) quoteString(s string) string {
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
