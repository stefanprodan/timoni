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
	"errors"
	"fmt"
	"regexp"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/load"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// GetConfigDoc extracts the config structure from the module.
func (b *ModuleBuilder) GetConfigDoc() ([][]string, error) {
	var value cue.Value

	cfg := &load.Config{
		ModuleRoot: b.moduleRoot,
		Package:    b.pkgName,
		Dir:        b.pkgPath,
		DataFiles:  true,
		Tags: []string{
			"name=" + b.name,
			"namespace=" + b.namespace,
		},
		TagVars: map[string]load.TagVar{
			"moduleVersion": {
				Func: func() (ast.Expr, error) {
					return ast.NewString(b.moduleVersion), nil
				},
			},
			"kubeVersion": {
				Func: func() (ast.Expr, error) {
					return ast.NewString(b.kubeVersion), nil
				},
			},
		},
	}

	modInstances := load.Instances([]string{}, cfg)
	if len(modInstances) == 0 {
		return nil, errors.New("no instances found")
	}

	modInstance := modInstances[0]
	if modInstance.Err != nil {
		return nil, fmt.Errorf("instance error: %w", modInstance.Err)
	}

	value = b.ctx.BuildInstance(modInstance)
	if value.Err() != nil {
		return nil, value.Err()
	}

	cfgValues := value.LookupPath(cue.ParsePath(apiv1.ConfigValuesSelector.String()))
	if cfgValues.Err() != nil {
		return nil, fmt.Errorf("lookup %s failed: %w", apiv1.ConfigValuesSelector, cfgValues.Err())
	}

	rows, err := iterateFields(cfgValues)
	if err != nil {
		return nil, err
	}

	return rows, nil
}

func iterateFields(v cue.Value) ([][]string, error) {
	var rows [][]string

	fields, err := v.Fields(
		cue.Optional(true),
		cue.Concrete(true),
		cue.Docs(true),
	)
	if err != nil {
		return nil, fmt.Errorf("Cue Fields Error: %w", err)
	}

	for fields.Next() {
		v := fields.Value()
		_, noDoc := hasNoDoc(v)

		if noDoc {
			continue
		}

		// We are chekcing if the field is a struct and not optional and is concrete before we iterate through it
		// this allows for definition of default values as full structs without generating output for each
		// field in the struct where it doesn't make sense e.g.
		//
		// - annotations?: {[string]: string}
		// - affinity: corev1.Affinity | *{nodeAffinity: requiredDuringSchedulingIgnoredDuringExecution: nodeSelectorTerms: [...]}
		if v.IncompleteKind() == cue.StructKind && !fields.IsOptional() && v.IsConcrete() {
			//if _, ok := v.Default(); v.IncompleteKind() == cue.StructKind && !fields.IsOptional() && ok {
			// Assume we want to use the field
			useField := true
			iRows, err := iterateFields(v)

			if err != nil {
				return nil, err
			}

			for _, row := range iRows {
				if len(row) > 0 {
					// If we have a row with more than 0 elements, we don't want to use the field and should use the child rows instead
					useField = false
					rows = append(rows, row)
				}
			}

			if useField {
				rows = append(rows, getField(v))
			}
		} else {
			rows = append(rows, getField(v))
		}
	}

	return rows, nil
}

func hasNoDoc(v cue.Value) (string, bool) {
	var noDoc bool
	var doc string

	for _, d := range v.Doc() {
		if line := len(d.List) - 1; line >= 0 {
			switch d.List[line].Text {
			case "// +nodoc":
				noDoc = true
				break
			}
		}

		doc += d.Text()
		doc = strings.ReplaceAll(doc, "\n", " ")
		doc = strings.ReplaceAll(doc, "+required", "")
		doc = strings.ReplaceAll(doc, "+optional", "")
	}

	return doc, noDoc
}

func getField(v cue.Value) []string {
	var row []string
	labelDomain := regexp.MustCompile(`^([a-zA-Z0-9-_.]+)?(".+")?$`)
	doc, noDoc := hasNoDoc(v)

	if !noDoc {
		fieldType := strings.ReplaceAll(fmt.Sprintf("%v", v), "\n", "")
		fieldType = strings.ReplaceAll(fieldType, "|", "\\|")
		fieldType = strings.ReplaceAll(fieldType, "\":", "\": ")
		fieldType = strings.ReplaceAll(fieldType, "\":[", "\": [")
		fieldType = strings.ReplaceAll(fieldType, "},", "}, ")

		if len(fieldType) == 0 {
			fieldType = " "
		}

		field := strings.Replace(v.Path().String(), "timoni.instance.config.", "", 1)
		match := labelDomain.FindStringSubmatch(field)

		row = append(row, fmt.Sprintf("`%s:`", strings.ReplaceAll(match[1], ".", ": ")+match[2]))
		row = append(row, fmt.Sprintf("`%s`", fieldType))
		row = append(row, fmt.Sprintf("%s", doc))
	}

	return row
}
