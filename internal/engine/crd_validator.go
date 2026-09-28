/*
Copyright 2026 Stefan Prodan

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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	ssautil "github.com/fluxcd/pkg/ssa/utils"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	apiextcel "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/listtype"
	schemaobjectmeta "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/objectmeta"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"
	apiextvalidation "k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
)

const (
	crdKind  = "CustomResourceDefinition"
	crdGroup = "apiextensions.k8s.io"
)

// CRDValidator validates custom resources against the schemas of their
// CustomResourceDefinitions with the Kubernetes API server admission checks:
// OpenAPI schema validation, embedded resource metadata, list map and set
// uniqueness, and the CEL validation rules (x-kubernetes-validations).
// Schemas are registered per group, version and kind, and the last
// registration for a kind version wins.
type CRDValidator struct {
	schemas map[schema.GroupVersionKind]*crdSchema
}

type crdSchema struct {
	structural   *apiextschema.Structural
	validator    apiextvalidation.SchemaValidator
	cel          *apiextcel.Validator
	pruneUnknown bool
	dropStatus   bool
	buildErr     error
}

// NewCRDValidator returns an empty CRDValidator.
func NewCRDValidator() *CRDValidator {
	return &CRDValidator{schemas: make(map[schema.GroupVersionKind]*crdSchema)}
}

// AddCRDs registers the schemas of the CustomResourceDefinitions found in
// the given objects, ignoring objects of any other kind.
func (v *CRDValidator) AddCRDs(objects []*unstructured.Unstructured) error {
	for _, object := range objects {
		if !IsCRD(object) {
			continue
		}
		if err := v.AddCRD(object); err != nil {
			return err
		}
	}
	return nil
}

// AddVendoredCRDs registers the schemas of the given CustomResourceDefinitions
// vendored with 'timoni mod vendor crd', skipping the kind versions already
// registered, so that the CRDs added with AddCRDs take precedence over the
// vendored schemas regardless of the registration order.
func (v *CRDValidator) AddVendoredCRDs(crds []*unstructured.Unstructured) error {
	for _, crd := range crds {
		if err := v.addCRD(crd, true); err != nil {
			return err
		}
	}
	return nil
}

// AddCRD registers the schema of every version of the given
// CustomResourceDefinition, replacing schemas previously registered
// for the same kind versions. A version whose schema cannot be compiled
// is registered with the compilation error, reported by Validate for
// every object of that kind version.
func (v *CRDValidator) AddCRD(crd *unstructured.Unstructured) error {
	return v.addCRD(crd, false)
}

// addCRD registers the schema of every version of the given
// CustomResourceDefinition, skipping the kind versions already registered
// when keepExisting is set.
func (v *CRDValidator) addCRD(crd *unstructured.Unstructured, keepExisting bool) error {
	group, versions, err := crdVersions(crd)
	if err != nil {
		return err
	}
	kind, _, _ := unstructured.NestedString(crd.Object, "spec", "names", "kind")
	preserveUnknown, _, _ := unstructured.NestedBool(crd.Object, "spec", "preserveUnknownFields")

	for _, ver := range versions {
		gvk := schema.GroupVersionKind{Group: group, Version: ver.name, Kind: kind}
		if _, registered := v.schemas[gvk]; registered && keepExisting {
			continue
		}
		s := &crdSchema{
			pruneUnknown: !preserveUnknown,
			dropStatus:   ver.statusSubresource,
		}
		v.schemas[gvk] = s

		structural, props, err := newStructuralSchema(ver.schema)
		if err != nil {
			s.buildErr = fmt.Errorf("invalid schema in CRD %s version %s: %w", crd.GetName(), ver.name, err)
			continue
		}
		if err := defaulting.PruneDefaults(structural); err != nil {
			s.buildErr = fmt.Errorf("invalid schema in CRD %s version %s: %w", crd.GetName(), ver.name, err)
			continue
		}
		validator, _, err := apiextvalidation.NewSchemaValidator(props)
		if err != nil {
			s.buildErr = fmt.Errorf("invalid schema in CRD %s version %s: %w", crd.GetName(), ver.name, err)
			continue
		}

		s.structural = structural
		s.validator = validator
		// The validator is nil when no rules are placed on structural nodes.
		s.cel = apiextcel.NewValidator(structural, true, celconfig.PerCallLimit)
	}
	return nil
}

// HasSchema returns true if a schema is registered for the given kind version.
func (v *CRDValidator) HasSchema(gvk schema.GroupVersionKind) bool {
	_, ok := v.schemas[gvk]
	return ok
}

// Validate checks the object against the schema registered for its kind
// version and returns one error per violation, sorted by field path. A copy
// of the object goes through the API server admission steps for a create
// request: the status is dropped when the CRD declares the status
// subresource, unknown fields are pruned unless the CRD preserves them, and
// the defaults declared in the schema are applied. The pruned fields are
// reported as violations. The CEL rules are evaluated only when the OpenAPI
// schema validation reports no blocking errors, and transition rules that
// reference oldSelf are skipped unless they set optionalOldSelf.
func (v *CRDValidator) Validate(ctx context.Context, object *unstructured.Unstructured) []error {
	s, ok := v.schemas[object.GroupVersionKind()]
	if !ok {
		return nil
	}
	if s.buildErr != nil {
		return []error{s.buildErr}
	}

	obj := object.DeepCopy().Object
	if s.dropStatus {
		delete(obj, "status")
	}
	var msgs []string
	if s.pruneUnknown {
		unknown := pruning.PruneWithOptions(obj, s.structural, true,
			apiextschema.UnknownFieldPathOptions{TrackUnknownFieldPaths: true})
		for _, path := range unknown {
			msgs = append(msgs, fmt.Sprintf("%s: unknown field", path))
		}
	}
	defaulting.PruneNonNullableNullsWithoutDefaults(obj, s.structural)
	defaulting.Default(obj, s.structural)

	schemaErrs := apiextvalidation.ValidateCustomResource(nil, obj, s.validator)
	schemaErrs = append(schemaErrs, schemaobjectmeta.Validate(ctx, nil, obj, s.structural, false)...)
	schemaErrs = append(schemaErrs, listtype.ValidateListSetsAndMaps(nil, s.structural, obj)...)
	msgs = append(msgs, formatFieldErrors(schemaErrs, false)...)

	if s.cel != nil {
		if hasBlockingErr(schemaErrs) {
			msgs = append(msgs, "some validation rules were not checked because the object was invalid; correct the existing errors to complete validation")
		} else {
			celErrs, _ := s.cel.Validate(ctx, nil, s.structural, obj, nil, celconfig.RuntimeCELCostBudget)
			msgs = append(msgs, formatFieldErrors(celErrs, true)...)
		}
	}

	errs := make([]error, 0, len(msgs))
	for _, msg := range msgs {
		errs = append(errs, errors.New(msg))
	}
	return errs
}

// ValidationError is a violation found by ValidateObjects in the
// custom resource identified by its kind, namespace and name.
type ValidationError struct {
	Object string
	Err    error
}

// Error returns the object identifier followed by the violation.
func (e ValidationError) Error() string {
	return e.Object + " " + e.Err.Error()
}

// Unwrap returns the violation.
func (e ValidationError) Unwrap() error {
	return e.Err
}

// ValidateObjects checks every object with a registered schema and returns
// one ValidationError per violation, in the order of the objects.
func (v *CRDValidator) ValidateObjects(ctx context.Context, objects []*unstructured.Unstructured) []ValidationError {
	var errs []ValidationError
	for _, object := range objects {
		if !v.HasSchema(object.GroupVersionKind()) {
			continue
		}
		for _, err := range v.Validate(ctx, object) {
			errs = append(errs, ValidationError{Object: ssautil.FmtUnstructured(object), Err: err})
		}
	}
	return errs
}

// hasBlockingErr returns true if the errors reported by the schema
// validation prevent the evaluation of the CEL rules.
func hasBlockingErr(errs field.ErrorList) bool {
	for _, err := range errs {
		switch err.Type {
		case field.ErrorTypeNotSupported, field.ErrorTypeRequired, field.ErrorTypeTooLong,
			field.ErrorTypeTooMany, field.ErrorTypeTypeInvalid:
			return true
		}
	}
	return false
}

// formatFieldErrors renders the field errors as '<path>: <message>' lines
// sorted by path and message. The message of a CEL rule error is the rule
// message, while the schema errors keep the API server error body, without
// the path repeated by the OpenAPI validator.
func formatFieldErrors(fieldErrs field.ErrorList, celRules bool) []string {
	type fieldMsg struct {
		path string
		msg  string
	}
	entries := make([]fieldMsg, 0, len(fieldErrs))
	for _, fe := range fieldErrs {
		var msg string
		if celRules && fe.Detail != "" {
			msg = fe.Detail
		} else {
			msg = strings.ReplaceAll(fe.ErrorBody(), fe.Field+" in body ", "")
		}
		if strings.Contains(msg, "\n") {
			msg = strings.Join(strings.Fields(msg), " ")
		}
		path := fe.Field
		if path == "<nil>" {
			path = ""
		}
		entries = append(entries, fieldMsg{path: path, msg: msg})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].path != entries[j].path {
			return entries[i].path < entries[j].path
		}
		return entries[i].msg < entries[j].msg
	})

	msgs := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.path == "" {
			msgs = append(msgs, e.msg)
			continue
		}
		msgs = append(msgs, fmt.Sprintf("%s: %s", e.path, e.msg))
	}
	return msgs
}

// IsCRD returns true if the object is a Kubernetes CustomResourceDefinition.
func IsCRD(object *unstructured.Unstructured) bool {
	return object.GetKind() == crdKind && object.GroupVersionKind().Group == crdGroup
}

// embeddedCRD returns the CRD manifest stored in the hidden field of a CUE
// package generated by Importer. Hidden fields with the same name that do not
// hold a CustomResourceDefinition belong to the package and are ignored.
func embeddedCRD(pkg cue.Value) (*unstructured.Unstructured, bool, error) {
	iter, err := pkg.Fields(cue.Hidden(true), cue.Definitions(false), cue.Optional(false))
	if err != nil {
		return nil, false, err
	}

	for iter.Next() {
		if iter.Selector().String() != crdField {
			continue
		}
		value := iter.Value()
		if !isCRDValue(value) {
			return nil, false, nil
		}

		data, err := value.MarshalJSON()
		if err != nil {
			return nil, false, err
		}
		objects, err := ssautil.ReadObjects(bytes.NewReader(data))
		if err != nil {
			return nil, false, err
		}
		if len(objects) != 1 {
			return nil, false, errors.New("expected a single CustomResourceDefinition with a name")
		}
		return objects[0], true, nil
	}
	return nil, false, nil
}

// isCRDValue returns true if the CUE value declares the apiVersion and kind
// of a CustomResourceDefinition.
func isCRDValue(value cue.Value) bool {
	apiVersion, err := value.LookupPath(cue.ParsePath("apiVersion")).String()
	if err != nil || apiVersion != crdGroup+"/v1" {
		return false
	}
	kind, err := value.LookupPath(cue.ParsePath("kind")).String()
	return err == nil && kind == crdKind
}

type crdVersion struct {
	name              string
	schema            map[string]any
	statusSubresource bool
}

// crdVersions returns the API group and the versions of the given CRD that
// declare an OpenAPI schema.
func crdVersions(crd *unstructured.Unstructured) (string, []crdVersion, error) {
	group, _, _ := unstructured.NestedString(crd.Object, "spec", "group")
	versions, _, err := unstructured.NestedSlice(crd.Object, "spec", "versions")
	if err != nil {
		return "", nil, fmt.Errorf("reading CRD %s spec.versions failed: %w", crd.GetName(), err)
	}

	result := make([]crdVersion, 0, len(versions))
	for i, item := range versions {
		version, ok := item.(map[string]any)
		if !ok {
			return "", nil, fmt.Errorf("CRD %s spec.versions[%d] must be an object", crd.GetName(), i)
		}
		name, _ := version["name"].(string)
		openAPISchema, found, err := unstructured.NestedMap(version, "schema", "openAPIV3Schema")
		if err != nil {
			return "", nil, fmt.Errorf("reading CRD %s version %s schema failed: %w", crd.GetName(), name, err)
		}
		if !found {
			continue
		}
		_, statusSubresource, _ := unstructured.NestedFieldNoCopy(version, "subresources", "status")
		result = append(result, crdVersion{
			name:              name,
			schema:            openAPISchema,
			statusSubresource: statusSubresource,
		})
	}
	return group, result, nil
}

// newStructuralSchema converts a CRD OpenAPI v3 schema into the internal
// JSONSchemaProps and the structural form used by the Kubernetes API server
// for validation, pruning, defaulting and CEL evaluation. The
// 'x-kubernetes-preserve-unknown-fields: false' markers, equivalent to
// undefined, are removed from the schema.
func newStructuralSchema(openAPISchema map[string]any) (*apiextschema.Structural, *apiextensions.JSONSchemaProps, error) {
	dropPreserveUnknownFieldsFalse(openAPISchema)
	data, err := json.Marshal(openAPISchema)
	if err != nil {
		return nil, nil, err
	}

	var v1Props apiextensionsv1.JSONSchemaProps
	if err := json.Unmarshal(data, &v1Props); err != nil {
		return nil, nil, err
	}

	var props apiextensions.JSONSchemaProps
	if err := apiextensionsv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(&v1Props, &props, nil); err != nil {
		return nil, nil, err
	}

	structural, err := apiextschema.NewStructural(&props)
	if err != nil {
		return nil, nil, err
	}
	return structural, &props, nil
}

// dropPreserveUnknownFieldsFalse removes the 'x-kubernetes-preserve-unknown-fields: false'
// entries at any depth of the schema.
func dropPreserveUnknownFieldsFalse(node any) {
	switch n := node.(type) {
	case map[string]any:
		if v, ok := n["x-kubernetes-preserve-unknown-fields"].(bool); ok && !v {
			delete(n, "x-kubernetes-preserve-unknown-fields")
		}
		for _, v := range n {
			dropPreserveUnknownFieldsFalse(v)
		}
	case []any:
		for _, v := range n {
			dropPreserveUnknownFieldsFalse(v)
		}
	}
}
