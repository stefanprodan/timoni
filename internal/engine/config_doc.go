/*
Copyright 2024 Stefan Prodan
SPDX-License-Identifier: Apache-2.0

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
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/token"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// ConfigField describes one field of a module's #Config schema.
type ConfigField struct {
	// Path is the CUE path of the field relative to the config root.
	Path cue.Path
	// Type is the CUE expression constraining the field, without its default.
	Type string
	// Default is the JSON encoding of the field's default or concrete value.
	Default string
	// Doc is the field documentation, one line per comment line.
	Doc string
	// Optional is true for fields declared with `?`.
	Optional bool
	// Required is true for fields declared with `!`.
	Required bool
	// NoDoc is true for fields commented with `// +nodoc`.
	NoDoc bool
}

// Key returns the field path in the `a: b: c:` form used by the config table,
// without the optional and required markers.
func (f ConfigField) Key() string {
	return strings.Join(plainLabels(f.Path), ": ") + ":"
}

// plainLabels returns the path labels without the `?` and `!` markers.
func plainLabels(p cue.Path) []string {
	selectors := p.Selectors()
	labels := make([]string, 0, len(selectors))
	for _, sel := range selectors {
		labels = append(labels, strings.TrimRight(sel.String(), "?!"))
	}
	return labels
}

// injectedConfigPaths are the #Config fields set by Timoni at apply time
// from the instance name, namespace, module version and Kubernetes version.
var injectedConfigPaths = []string{
	"kubeVersion",
	"clusterVersion",
	"moduleVersion",
	"metadata.name",
	"metadata.namespace",
}

// GetConfigDoc extracts the fields of the module's #Config schema from
// the built module value. A field is listed when it is declared in the
// module itself (outside cue.mod) or carries a default, and its
// children are listed when it is a struct without a default. The fields injected by Timoni at apply time are skipped.
func (b *ModuleBuilder) GetConfigDoc(value cue.Value) ([]ConfigField, error) {
	root := value.LookupPath(cue.ParsePath(apiv1.ValuesSelector.String()))
	if root.Err() != nil {
		return nil, fmt.Errorf("lookup %s failed: %w", apiv1.ValuesSelector, root.Err())
	}

	var fields []ConfigField
	var walk func(v cue.Value, path cue.Path) error
	walk = func(v cue.Value, path cue.Path) error {
		iter, err := v.Fields(cue.Optional(true), cue.Hidden(false), cue.Definitions(false))
		if err != nil {
			return err
		}
		for iter.Next() {
			fv := iter.Value()
			sel := iter.Selector()
			selectors := append(slices.Clone(path.Selectors()), sel)
			fieldPath := cue.MakePath(selectors...)
			if slices.Contains(injectedConfigPaths, strings.Join(plainLabels(fieldPath), ".")) {
				continue
			}

			optional := sel.ConstraintType() == cue.OptionalConstraint
			required := sel.ConstraintType() == cue.RequiredConstraint
			local := b.localConjuncts(fv)
			def, hasDef := fv.Default()
			ownDefault := hasOwnDefault(fv, local)
			// A struct inherits a default from an embedded disjunction and
			// an optional field from its type; only a default declared on
			// the field itself counts for them.
			if hasDef && !ownDefault && (optional || fv.IncompleteKind() == cue.StructKind) {
				hasDef = false
			}

			if len(local) == 0 && (optional || !hasDef) {
				continue
			}

			doc, noDoc := b.fieldDoc(fv)
			f := ConfigField{
				Path:     fieldPath,
				Type:     typeExpr(fv, local),
				Doc:      doc,
				Optional: optional,
				Required: required,
				NoDoc:    noDoc,
			}
			switch {
			case hasDef:
				f.Default = cueValue(def)
			case !optional && fv.IsConcrete() && fv.Kind() != cue.StructKind && fv.Kind() != cue.ListKind:
				// A constant field: the literal is both the type and the value.
				f.Type = cueValue(fv)
				f.Default = f.Type
			}
			fields = append(fields, f)

			if !hasDef && fv.IncompleteKind() == cue.StructKind {
				if err := walk(fv, fieldPath); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := walk(root, cue.MakePath()); err != nil {
		return nil, err
	}
	return fields, nil
}

// localConjuncts returns the source expressions of the value's conjuncts
// that are declared in the module files, excluding the ones coming from
// the packages vendored under cue.mod.
func (b *ModuleBuilder) localConjuncts(v cue.Value) []ast.Expr {
	var exprs []ast.Expr
	add := func(cv cue.Value) {
		if !b.isLocalPos(cv.Pos()) {
			return
		}
		if e := sourceExpr(cv); e != nil {
			exprs = append(exprs, e)
		}
	}

	op, args := v.Expr()
	if op == cue.AndOp {
		for _, a := range args {
			add(a)
		}
		return exprs
	}
	add(v)
	return exprs
}

// isLocalPos reports whether the position belongs to a module file
// outside the cue.mod directory.
func (b *ModuleBuilder) isLocalPos(pos token.Pos) bool {
	file := pos.Filename()
	if file == "" {
		return false
	}
	for _, root := range b.localRoots() {
		rel, err := filepath.Rel(root, file)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return rel != "cue.mod" && !strings.HasPrefix(rel, "cue.mod"+string(filepath.Separator))
	}
	return false
}

// localRoots returns the absolute module root as given and with
// symlinks resolved, since the CUE source positions may record either.
func (b *ModuleBuilder) localRoots() []string {
	root := b.moduleRoot
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	roots := []string{root}
	if resolved, err := filepath.EvalSymlinks(root); err == nil && resolved != root {
		roots = append(roots, resolved)
	}
	return roots
}

// sourceExpr returns the expression behind a value's source node,
// unwrapping field declarations to their value.
func sourceExpr(v cue.Value) ast.Expr {
	switch s := v.Source().(type) {
	case *ast.Field:
		return s.Value
	case ast.Expr:
		return s
	}
	return nil
}

// typeExpr renders the field type from its local conjuncts, dropping
// default markers and struct literals, and falls back to the CUE kind.
func typeExpr(v cue.Value, local []ast.Expr) string {
	var parts []string
	for _, e := range local {
		e = stripDefaults(e)
		e = stripStructs(e)
		if e == nil {
			continue
		}
		b, err := format.Node(e, format.Simplify())
		if err != nil {
			continue
		}
		s := singleLine(string(b))
		if s != "" && !slices.Contains(parts, s) {
			parts = append(parts, s)
		}
	}
	if len(parts) > 1 {
		parts = slices.DeleteFunc(parts, func(s string) bool {
			return isLiteral(s)
		})
	}
	if len(parts) == 0 {
		return v.IncompleteKind().String()
	}
	return strings.Join(parts, " & ")
}

// singleLine joins a formatted CUE expression into one line,
// separating the struct fields and list elements with commas.
func singleLine(src string) string {
	if strings.Contains(src, `"""`) || strings.Contains(src, "'''") {
		return strings.TrimSpace(src)
	}
	var out []string
	for _, line := range strings.Split(src, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		if len(out) > 0 {
			prev := out[len(out)-1]
			if !strings.HasSuffix(prev, "{") && !strings.HasSuffix(prev, "[") &&
				!strings.HasPrefix(line, "}") && !strings.HasPrefix(line, "]") {
				out[len(out)-1] = prev + ","
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, " ")
}

// isLiteral reports whether the rendered expression is a concrete
// string, number or bool literal rather than a type or constraint.
func isLiteral(s string) bool {
	if s == "true" || s == "false" || s == "null" {
		return true
	}
	if strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"") {
		return true
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// hasOwnDefault reports whether the default is declared on the field
// itself as a disjunction with a default marker, checking the field
// source when available and the local conjuncts otherwise.
func hasOwnDefault(v cue.Value, local []ast.Expr) bool {
	exprs := local
	if src := sourceExpr(v); src != nil {
		exprs = []ast.Expr{src}
	}
	for _, e := range exprs {
		if hasDefaultMarker(e) {
			return true
		}
	}
	return false
}

// hasDefaultMarker reports whether the expression contains a `*`
// default marker outside of struct and list literals.
func hasDefaultMarker(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.UnaryExpr:
		return x.Op == token.MUL
	case *ast.BinaryExpr:
		return hasDefaultMarker(x.X) || hasDefaultMarker(x.Y)
	case *ast.ParenExpr:
		return hasDefaultMarker(x.X)
	}
	return false
}

// stripDefaults removes the `*default |` alternative from a disjunction.
func stripDefaults(e ast.Expr) ast.Expr {
	switch x := e.(type) {
	case *ast.UnaryExpr:
		if x.Op == token.MUL {
			return nil
		}
	case *ast.BinaryExpr:
		if x.Op != token.OR && x.Op != token.AND {
			return e
		}
		l, r := stripDefaults(x.X), stripDefaults(x.Y)
		switch {
		case l == nil:
			return r
		case r == nil:
			return l
		}
		return &ast.BinaryExpr{X: l, Op: x.Op, Y: r}
	case *ast.ParenExpr:
		return paren(stripDefaults(x.X))
	}
	return e
}

// paren wraps a binary expression in parentheses and returns any
// other expression unchanged.
func paren(e ast.Expr) ast.Expr {
	if _, ok := e.(*ast.BinaryExpr); ok {
		return &ast.ParenExpr{X: e}
	}
	return e
}

// stripStructs removes struct literals from a conjunction so that
// only the named types, pattern constraints and other constraints remain.
func stripStructs(e ast.Expr) ast.Expr {
	switch x := e.(type) {
	case *ast.StructLit:
		for _, d := range x.Elts {
			f, ok := d.(*ast.Field)
			if !ok {
				return nil
			}
			if _, ok := f.Label.(*ast.ListLit); !ok {
				return nil
			}
		}
		return e
	case *ast.BinaryExpr:
		if x.Op != token.AND {
			return e
		}
		l, r := stripStructs(x.X), stripStructs(x.Y)
		switch {
		case l == nil:
			return r
		case r == nil:
			return l
		}
		return &ast.BinaryExpr{X: l, Op: x.Op, Y: r}
	case *ast.ParenExpr:
		return paren(stripStructs(x.X))
	}
	return e
}

// fieldDoc returns the documentation written in the module for the
// field and reports whether the field is marked with `// +nodoc`.
// The comments inherited from imported definitions are left out.
func (b *ModuleBuilder) fieldDoc(v cue.Value) (string, bool) {
	var lines []string
	var noDoc bool
	for _, d := range v.Doc() {
		if line := len(d.List) - 1; line >= 0 && d.List[line].Text == "// +nodoc" {
			noDoc = true
		}
		if !b.isLocalPos(d.Pos()) {
			continue
		}
		text := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(d.Text()), "+nodoc"))
		lines = append(lines, text)
	}
	doc := strings.Join(lines, "\n")
	doc = strings.ReplaceAll(doc, "+required", "")
	doc = strings.ReplaceAll(doc, "+optional", "")
	return strings.TrimSpace(doc), noDoc
}

// cueValue renders a concrete CUE value as a single-line CUE literal
// without attributes, or an empty string when the value is not concrete.
func cueValue(v cue.Value) string {
	if err := v.Validate(cue.Concrete(true)); err != nil {
		return ""
	}
	node := v.Syntax(cue.Final(), cue.Concrete(true))
	node = astutil.Apply(node, func(c astutil.Cursor) bool {
		switch x := c.Node().(type) {
		case *ast.Attribute:
			c.Delete()
		case *ast.Field:
			x.Attrs = nil
		}
		return true
	}, nil)
	b, err := format.Node(node, format.Simplify())
	if err != nil {
		return ""
	}
	return singleLine(string(b))
}

// FormatConfigCUE renders the config fields as a CUE definition named
// #Config, preserving the field documentation, the optional and
// required markers, the defaults and the type constraints.
func FormatConfigCUE(fields []ConfigField) (string, error) {
	root := &configNode{}
	for _, f := range fields {
		n := root
		for _, sel := range f.Path.Selectors() {
			label := strings.TrimRight(sel.String(), "?!")
			child := n.children[label]
			if child == nil {
				child = &configNode{label: label}
				if n.children == nil {
					n.children = map[string]*configNode{}
				}
				n.children[label] = child
				n.order = append(n.order, label)
			}
			n = child
		}
		field := f
		n.field = &field
	}

	var sb strings.Builder
	sb.WriteString("#Config: {\n")
	root.write(&sb)
	sb.WriteString("}\n")

	out, err := format.Source([]byte(sb.String()), format.Simplify())
	if err != nil {
		return "", fmt.Errorf("formatting the config failed: %w", err)
	}
	return string(out), nil
}

// configNode is a tree node used to nest the config fields by path.
type configNode struct {
	label    string
	field    *ConfigField
	children map[string]*configNode
	order    []string
}

func (n *configNode) write(sb *strings.Builder) {
	for _, label := range n.order {
		child := n.children[label]
		f := child.field
		for _, line := range strings.Split(f.Doc, "\n") {
			if line != "" {
				sb.WriteString("// " + line + "\n")
			}
		}

		key := label
		switch {
		case f.Optional:
			key += "?"
		case f.Required:
			key += "!"
		}

		typ := f.Type
		if f.Default != "" && f.Default != typ {
			typ = "*" + f.Default + " | " + typ
		}

		switch {
		case len(child.order) == 0:
			sb.WriteString(key + ": " + typ + "\n")
		case typ == "struct":
			sb.WriteString(key + ": {\n")
			child.write(sb)
			sb.WriteString("}\n")
		default:
			sb.WriteString(key + ": " + typ + " & {\n")
			child.write(sb)
			sb.WriteString("}\n")
		}
	}
}
