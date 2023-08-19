package engine

import (
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/encoding/openapi"
	"cuelang.org/go/encoding/yaml"
)

// YamlCRDToCueIR converts a byte slice containing one or more YAML-encoded
// CustomResourceDefinitions into a slice of [IntermediateCRD].
//
// This function preserves the ordering of schemas declared in the input YAML in
// the resulting [IntermediateCRD.Schemas] field.
func YamlCRDToCueIR(ctx *cue.Context, b []byte) ([]*IntermediateCRD, error) {
	if ctx == nil {
		ctx = cuecontext.New()
	}

	// The filename provided here is only used in error messages
	yf, err := yaml.Extract("crd.yaml", b)
	if err != nil {
		return nil, fmt.Errorf("input is not valid yaml: %w", err)
	}
	crdv := ctx.BuildFile(yf)

	var all []cue.Value
	switch crdv.IncompleteKind() {
	case cue.StructKind:
		all = append(all, crdv)
	case cue.ListKind:
		iter, _ := crdv.List()
		for iter.Next() {
			all = append(all, iter.Value())
		}
	default:
		return nil, fmt.Errorf("input does not appear to be one or multiple CRDs: %s", crdv)
	}

	// TODO should this check that individual CRD inputs are valid?
	ret := make([]*IntermediateCRD, 0, len(all))
	for _, crd := range all {
		cc, err := convertCRD(crd)
		if err != nil {
			return nil, err
		}
		ret = append(ret, cc)
	}

	return ret, nil
}

// IntermediateCRD is an intermediate representation of CRD YAML. It contains the original CRD YAML input,
// a subset of useful naming-related fields, and an extracted list of the version schemas in the CRD,
// having been converted from OpenAPI to CUE.
type IntermediateCRD struct {
	// The original unmodified CRD YAML, after conversion to a cue.Value.
	Original cue.Value
	Props    struct {
		Spec struct {
			Group string `json:"group"`
			Names struct {
				Kind     string `json:"kind"`
				ListKind string `json:"listKind"`
				Plural   string `json:"plural"`
				Singular string `json:"singular"`
			} `json:"names"`
			Scope string `json:"scope"`
		} `json:"spec"`
	}

	// All the schemas in the original CRD, converted to CUE representation.
	Schemas []VersionedSchema
}

// VersionedSchema is an intermediate form of a single versioned schema from a CRD
// (an element in `spec.versions`), converted to CUE.
type VersionedSchema struct {
	// The contents of the `spec.versions[].name`
	Version string
	// The contents of `spec.versions[].schema.openAPIV3Schema`, after conversion of the OpenAPI
	// schema to native CUE constraints.
	Schema cue.Value
}

func convertCRD(crd cue.Value) (*IntermediateCRD, error) {
	cc := &IntermediateCRD{
		Schemas: make([]VersionedSchema, 0),
	}

	err := crd.Decode(&cc.Props)
	if err != nil {
		return nil, fmt.Errorf("error decoding crd props into Go struct: %w", err)
	}
	// shorthand
	kname := cc.Props.Spec.Names.Kind

	vlist := crd.LookupPath(cue.ParsePath("spec.versions"))
	if !vlist.Exists() {
		return nil, fmt.Errorf("crd versions list is absent")
	}
	iter, err := vlist.List()
	if err != nil {
		return nil, fmt.Errorf("crd versions field is not a list")
	}

	ctx := crd.Context()
	shell := ctx.CompileString(fmt.Sprintf(`
		openapi: "3.0.0",
		info: {
			title: "dummy",
			version: "1.0.0",
		}
		components: schemas: %s: _
	`, kname))
	schpath := cue.ParsePath("components.schemas." + kname)
	defpath := cue.MakePath(cue.Def(kname))

	// The CUE stdlib openapi encoder expects a whole openapi document, and then
	// operates on schema elements defined within #/components/schema. Each
	// versions[].schema.openAPIV3Schema within a CRD is ~equivalent to a single
	// element under #/components/schema, as k8s does not allow CRD schemas to
	// contain any kind of external references.
	//
	// So, for each schema.openAPIV3Schema, we wrap it in an openapi document
	// structure, convert it to CUE, then appends it into the [IntermediateCRD.Schemas] slice.
	var i int
	for iter.Next() {
		val := iter.Value()
		ver, err := val.LookupPath(cue.ParsePath("name")).String()
		if err != nil {
			return nil, fmt.Errorf("unreachable? error getting version field for versions element at index %d: %w", i, err)
		}
		i++

		doc := shell.FillPath(schpath, val.LookupPath(cue.ParsePath("schema.openAPIV3Schema")))
		of, err := openapi.Extract(doc, &openapi.Config{})
		if err != nil {
			return nil, fmt.Errorf("could not convert schema for version %s to CUE: %w", ver, err)
		}
		// first, extract and get the schema handle itself
		extracted := ctx.BuildFile(of)
		// then unify with our desired base constraints
		var ns string
		if cc.Props.Spec.Scope != "Namespaced" {
			ns = "?"
		}
		sch := extracted.FillPath(defpath, (ctx.CompileString(fmt.Sprintf(`
			apiVersion: "%s/%s"
			kind: "%s"

			metadata: {
				name:         string
				namespace%s:  string
				labels?:      [string]: string
				annotations?: [string]: string
			}
		`, cc.Props.Spec.Group, ver, kname, ns))))
		// next, go back to an AST because it's easier to manipulate references there
		schast := sch.Syntax(cue.All(), cue.Docs(true)).(*ast.File)

		// First pass, remove all ellipses so we default to closedness, in
		// keeping with the spirit of structural schema.
		// TODO add handling for k8s permitted exceptions, like x-kubernetes-preserve-unknown-fields
		ast.Walk(schast, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.StructLit:
				// Stuff could get weird with messing with node pointers while walking the tree,
				// which is why
				newlist := make([]ast.Decl, 0, len(x.Elts))
				for _, elt := range x.Elts {
					if _, is := elt.(*ast.Ellipsis); !is {
						newlist = append(newlist, elt)
					}
				}
				x.Elts = newlist
			}
			return true
		}, nil)
		specf, statusf := new(ast.Field), new(ast.Field)
		astutil.Apply(schast, func(cursor astutil.Cursor) bool {
			switch x := cursor.Node().(type) {
			case *ast.Field:
				if str, _, err := ast.LabelName(x.Label); err == nil {
					switch str {
					// Grab pointers to the spec and status fields, and replace with ref
					case "spec":
						*specf = *x
						specref := &ast.Field{
							Label: ast.NewIdent("spec"),
							Value: ast.NewIdent("#" + kname + "Spec"),
						}
						astutil.CopyComments(specref, x)
						cursor.Replace(specref)
						return false
					case "status":
						*statusf = *x
						statusref := &ast.Field{
							Label: ast.NewIdent("status"),
							Value: ast.NewIdent("#" + kname + "Status"),
						}
						astutil.CopyComments(statusref, x)
						cursor.Replace(statusref)
						return false
					case "metadata":
						// Avoid walking other known subtrees
						return false
					case "info":
						cursor.Delete()
					}
				}
			}
			return true
		}, nil)
		specd := &ast.Field{
			Label: ast.NewIdent("#" + kname + "Spec"),
			Value: specf.Value,
		}
		astutil.CopyComments(specd, specf)
		schast.Decls = append(schast.Decls, specd)

		if statusf != nil {
			statusd := &ast.Field{
				Label: ast.NewIdent("#" + kname + "Status"),
				Value: statusf.Value,
			}
			astutil.CopyComments(statusd, statusf)
			schast.Decls = append(schast.Decls, statusd)
		}

		// Then build back to a cue.Value again for the return
		cc.Schemas = append(cc.Schemas, VersionedSchema{
			Version: ver,
			Schema:  ctx.BuildFile(schast),
		})
	}
	return cc, nil
}
