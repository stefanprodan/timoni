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
	"os"

	"cuelang.org/go/cue"
)

// ExtractValueFromFile compiles the given file and
// returns the CUE value that matches the given expression.
func ExtractValueFromFile(ctx *cue.Context, filePath, expr string) (cue.Value, error) {
	vData, err := os.ReadFile(filePath)
	if err != nil {
		return cue.Value{}, err
	}
	return ExtractValueFromBytes(ctx, vData, expr)
}

func ExtractValueFromBytes(ctx *cue.Context, data []byte, expr string) (cue.Value, error) {
	vObj := ctx.CompileBytes(data)
	if vObj.Err() != nil {
		return cue.Value{}, vObj.Err()
	}

	value := vObj.LookupPath(cue.ParsePath(expr))
	if value.Err() != nil {
		return cue.Value{}, vObj.Err()
	}

	return value, nil
}

// MergeValue merges the given overlay on top of the base CUE value.
// New fields from the overlay are added to the base and
// existing fields are overridden with the overlay values.
func MergeValue(overlay, base cue.Value) (cue.Value, error) {
	r, _ := mergeValue(overlay, base)
	return r, nil
}

func mergeValue(overlay, base cue.Value) (cue.Value, bool) {
	switch base.IncompleteKind() {
	case cue.StructKind:
		return mergeStruct(overlay, base)
	case cue.ListKind:
		return mergeList(overlay, base)
	}
	return overlay, true
}

func mergeStruct(overlay, base cue.Value) (cue.Value, bool) {
	out := overlay
	iter, _ := base.Fields(
		cue.Concrete(true),
		cue.Attributes(true),
		cue.Definitions(true),
		cue.Hidden(true),
		cue.Optional(true),
		cue.Docs(true),
	)

	for iter.Next() {
		s := iter.Selector()
		p := cue.MakePath(s)
		r := overlay.LookupPath(p)
		if r.Exists() {
			v, ok := mergeValue(r, iter.Value())
			if ok {
				out = out.FillPath(p, v)
			}
		} else {
			out = out.FillPath(p, iter.Value())
		}
	}

	return out, true
}

func mergeList(overlay, base cue.Value) (cue.Value, bool) {
	ctx := base.Context()

	ri, _ := overlay.List()
	ti, _ := base.List()

	var out []cue.Value
	for ri.Next() && ti.Next() {
		r, ok := mergeValue(ri.Value(), ti.Value())
		if ok {
			out = append(out, r)
		}
	}
	return ctx.NewList(out...), true
}
