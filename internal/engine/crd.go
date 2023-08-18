package engine

import (
	"fmt"

	"cuelang.org/go/cue"
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

	// TODO should this validate that individual CRD inputs are valid?
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

	vlist := crd.LookupPath(cue.ParsePath("spec.versions"))
	if !vlist.Exists() {
		return nil, fmt.Errorf("crd versions list is absent")
	}
	iter, err := vlist.List()
	if err != nil {
		return nil, fmt.Errorf("crd versions field is not a list")
	}

	ctx := crd.Context()
	shell := ctx.CompileString(`
		openapi: "3.0.0",
		info: {
			title: "dummy",
			version: "1.0.0",
		}
		components: schemas: thedef: _
	`)
	schpath := cue.ParsePath("components.schemas.thedef")
	defpath := cue.MakePath(cue.Def("thedef"))

	// The CUE stdlib openapi encoder expects a whole openapi document, and then
	// operates on schema elements defined within #/components/schema. Each
	// versions[].schema.openAPIV3Schema within a CRD is ~equivalent to a single
	// element under #/components/schema, as k8s does not allow CRD schemas to
	// contain any kind of external references.
	//
	// So, for each schema.openAPIV3Schema, we wrap it in an openapi document
	// structure, convert it to CUE, then merge it back into place at its
	// original position.
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
		sch := ctx.BuildFile(of)
		cc.Schemas = append(cc.Schemas, VersionedSchema{
			Version: ver,
			Schema:  sch.LookupPath(defpath),
		})

		// Additional massaging of converted schemas should be done here. Lots
		// could be done. One example: apiVersion field appears to be coming
		// through conversion as optional.
	}
	return cc, nil
}
