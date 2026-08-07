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
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

const (
	// testFileSuffix is the suffix of the CUE files holding a module's test cases.
	// The CUE loader excludes these files from regular package loads, so they are
	// invisible to build, apply and vet, and are only read when Tests is enabled.
	testFileSuffix = "_test.cue"

	// defaultTestName is the instance name a test case is built with
	// unless the case overrides it.
	defaultTestName = "test"

	// defaultTestNamespace is the instance namespace a test case is built with
	// unless the case overrides it.
	defaultTestNamespace = "test"
)

// caseFields are the fields a test case may declare. Hidden fields are not
// part of this set: they are excluded from a case's field set and are where
// helpers and the values under test belong.
var caseFields = []apiv1.Selector{
	apiv1.TestNameSelector,
	apiv1.TestNamespaceSelector,
	apiv1.TestModuleVersionSelector,
	apiv1.TestKubeVersionSelector,
	apiv1.TestValuesSelector,
	apiv1.TestObjectsSelector,
	apiv1.TestAssertSelector,
}

// TestCase is a single test case declared under the 'cases' field
// of a module's *_test.cue file.
type TestCase struct {
	// Name is the case name as declared in CUE.
	Name string

	// Dir is the directory holding the CUE package the case was declared in,
	// relative to the module root.
	Dir string

	// value holds the case declaration, before the rendered objects are filled in.
	value cue.Value
}

// TestResult is the outcome of running a TestCase.
type TestResult struct {
	// Case is the test case that produced this result.
	Case TestCase

	// Err is nil when the case passed, and holds the reason it failed otherwise.
	Err error
}

// Passed reports whether the test case succeeded.
func (r TestResult) Passed() bool {
	return r.Err == nil
}

// ModuleTester runs the test cases declared in a module's *_test.cue files.
// Each case is checked against a build made with its own inputs, so a case
// cannot influence the outcome of another. A tester is not safe for concurrent
// use.
type ModuleTester struct {
	ctx        *cue.Context
	moduleRoot string
	pkgName    string
	builds     map[string]*moduleBuild
}

// NewModuleTester creates a ModuleTester for the module at the given root.
func NewModuleTester(ctx *cue.Context, moduleRoot, pkgName string) *ModuleTester {
	if ctx == nil {
		ctx = cuecontext.New()
	}
	return &ModuleTester{
		ctx:        ctx,
		moduleRoot: moduleRoot,
		pkgName:    pkgName,
		builds:     make(map[string]*moduleBuild),
	}
}

// LoadCases finds the module's *_test.cue files, loads the CUE packages holding
// them, and returns the test cases declared under their 'cases' field.
// The cases are returned sorted by directory and name so that runs are reproducible.
func (t *ModuleTester) LoadCases() ([]TestCase, error) {
	dirs, err := t.testDirs()
	if err != nil {
		return nil, err
	}

	var cases []TestCase
	for _, dir := range dirs {
		found, err := t.loadCasesFromDir(dir)
		if err != nil {
			return nil, fmt.Errorf("loading tests from %s failed: %w", dir, err)
		}
		cases = append(cases, found...)
	}

	sort.SliceStable(cases, func(i, j int) bool {
		if cases[i].Dir != cases[j].Dir {
			return cases[i].Dir < cases[j].Dir
		}
		return cases[i].Name < cases[j].Name
	})

	return cases, nil
}

// testDirs returns the module directories containing at least one *_test.cue file,
// relative to the module root. The CUE module metadata directory is skipped.
func (t *ModuleTester) testDirs() ([]string, error) {
	seen := make(map[string]bool)
	var dirs []string

	err := filepath.WalkDir(t.moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "cue.mod" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), testFileSuffix) {
			return nil
		}

		dir := filepath.Dir(path)
		rel, err := filepath.Rel(t.moduleRoot, dir)
		if err != nil {
			return err
		}
		if !seen[rel] {
			seen[rel] = true
			dirs = append(dirs, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(dirs)
	return dirs, nil
}

// loadCasesFromDir loads the CUE package in the given module directory with test
// files included, and extracts the cases declared under its 'cases' field.
func (t *ModuleTester) loadCasesFromDir(dir string) ([]TestCase, error) {
	cfg := &load.Config{
		AcceptLegacyModules: true,
		Dir:                 t.moduleRoot,
		DataFiles:           true,
		Tests:               true,
	}

	instances := load.Instances([]string{"./" + filepath.ToSlash(dir)}, cfg)
	if len(instances) == 0 {
		return nil, errors.New("no instances found")
	}

	instance := instances[0]
	if instance.Err != nil {
		return nil, instance.Err
	}

	// The build value is not checked for errors here: a case that asserts a
	// value is rejected holds an intentional bottom, which would otherwise be
	// reported as a failure to load the package. Errors are instead scoped to
	// each case when it runs.
	value := t.ctx.BuildInstance(instance)

	cases := value.LookupPath(cue.ParsePath(apiv1.TestCasesSelector.String()))
	if !cases.Exists() {
		// A package may hold test helpers without declaring any case.
		return nil, nil
	}

	iter, err := cases.Fields()
	if err != nil {
		return nil, fmt.Errorf("lookup %s failed: %w", apiv1.TestCasesSelector, err)
	}

	var result []TestCase
	for iter.Next() {
		result = append(result, TestCase{
			Name:  iter.Selector().Unquoted(),
			Dir:   dir,
			value: iter.Value(),
		})
	}

	return result, nil
}

// Run builds the module with the case's values and checks its expectations
// against the resulting Kubernetes objects.
func (t *ModuleTester) Run(tc TestCase) TestResult {
	if err := checkCaseFields(tc.value); err != nil {
		return TestResult{Case: tc, Err: err}
	}

	rendered := t.render(tc)
	if rendered.err != nil {
		return TestResult{Case: tc, Err: rendered.err}
	}

	if err := checkExpectedObjectsExist(tc.value, rendered.objects); err != nil {
		return TestResult{Case: tc, Err: err}
	}

	if err := checkExpectedFields(tc.value, rendered.value); err != nil {
		return TestResult{Case: tc, Err: err}
	}

	// Fill the rendered objects into the case. The objects are concrete data, so
	// unifying an expectation with them is a comparison rather than a refinement:
	// a field the module leaves as a disjunction with a default would otherwise
	// absorb the expected value instead of conflicting with it.
	// The filled value is not checked for errors as a whole: a case that asserts
	// a value is rejected holds an intentional bottom. Only the expectations and
	// the assertions are validated.
	filled := tc.value.FillPath(cue.ParsePath(apiv1.TestObjectsSelector.String()), rendered.value)

	if err := checkObjects(filled); err != nil {
		return TestResult{Case: tc, Err: err}
	}

	if err := checkAssertions(filled); err != nil {
		return TestResult{Case: tc, Err: err}
	}

	return TestResult{Case: tc}
}

// objectKey returns the identifier a test case addresses an object by.
// The unqualified form is the '<kind>/<namespace>/<name>' identifier Timoni
// prints for an object everywhere else, with the namespace omitted when the
// object is cluster-scoped. The qualified form carries the API group, and is
// used for the objects that would otherwise share an identifier.
func objectKey(object *unstructured.Unstructured, qualified bool) string {
	kind := object.GetKind()
	if group := object.GroupVersionKind().Group; qualified && group != "" {
		kind = fmt.Sprintf("%s.%s", kind, group)
	}

	if namespace := object.GetNamespace(); namespace != "" {
		return fmt.Sprintf("%s/%s/%s", kind, namespace, object.GetName())
	}

	return fmt.Sprintf("%s/%s", kind, object.GetName())
}

// buildInputs are the inputs a test case builds the module with.
type buildInputs struct {
	name          string
	namespace     string
	moduleVersion string
	kubeVersion   string
	values        cue.Value
}

// key identifies a set of build inputs. The values are keyed by their CUE text,
// the form the module builder overlays them as, so that two cases configured
// the same way share a key and any difference between them does not.
func (in buildInputs) key() string {
	var values string
	if in.values.Exists() {
		values = fmt.Sprintf("%v", in.values)
	}

	return strings.Join([]string{
		in.name,
		in.namespace,
		in.moduleVersion,
		in.kubeVersion,
		values,
	}, "\x00")
}

// caseInputs reads the build inputs a test case declares, falling back to the
// defaults for the ones it leaves out.
func caseInputs(tc TestCase) (buildInputs, error) {
	var inputs buildInputs
	var err error

	if inputs.name, err = stringField(tc.value, apiv1.TestNameSelector, defaultTestName); err != nil {
		return inputs, err
	}

	if inputs.namespace, err = stringField(tc.value, apiv1.TestNamespaceSelector, defaultTestNamespace); err != nil {
		return inputs, err
	}

	if inputs.moduleVersion, err = stringField(tc.value, apiv1.TestModuleVersionSelector, ""); err != nil {
		return inputs, err
	}

	if inputs.kubeVersion, err = stringField(tc.value, apiv1.TestKubeVersionSelector, ""); err != nil {
		return inputs, err
	}

	inputs.values = tc.value.LookupPath(cue.ParsePath(apiv1.TestValuesSelector.String()))

	return inputs, nil
}

// moduleBuild is the outcome of building the module with one set of inputs.
type moduleBuild struct {
	// objects are the rendered Kubernetes objects, keyed by test identifier.
	objects map[string]any

	// value holds the same objects encoded as CUE, ready to be unified
	// with a case's expectations.
	value cue.Value

	// err is nil when the module built, and holds the reason it did not otherwise.
	err error
}

// render returns the objects the module renders for the given case. A build is
// a function of its inputs alone, so the cases that declare the same ones are
// served the build made for the first of them, failures included.
func (t *ModuleTester) render(tc TestCase) *moduleBuild {
	inputs, err := caseInputs(tc)
	if err != nil {
		return &moduleBuild{err: err}
	}

	key := inputs.key()
	if cached, ok := t.builds[key]; ok {
		return cached
	}

	build := t.build(inputs)
	t.builds[key] = build

	return build
}

// build compiles the module with the given inputs and encodes the Kubernetes
// objects it renders.
func (t *ModuleTester) build(inputs buildInputs) *moduleBuild {
	builder := NewModuleBuilder(t.ctx, inputs.name, inputs.namespace, t.moduleRoot, t.pkgName)
	builder.SetVersionInfo(inputs.moduleVersion, inputs.kubeVersion)

	if err := builder.OverlaySchemaFile(); err != nil {
		return &moduleBuild{err: err}
	}

	if inputs.values.Exists() {
		if err := builder.OverlayValuesFileWithDefaults(inputs.values); err != nil {
			return &moduleBuild{err: fmt.Errorf("invalid values: %w", err)}
		}
	}

	buildResult, err := builder.Build()
	if err != nil {
		return &moduleBuild{err: fmt.Errorf("build failed: %w", err)}
	}

	applySets, err := builder.GetApplySets(buildResult)
	if err != nil {
		return &moduleBuild{err: fmt.Errorf("build failed: %w", err)}
	}

	var objects []*unstructured.Unstructured
	for _, set := range applySets {
		objects = append(objects, set.Objects...)
	}

	keyed, err := keyObjects(objects)
	if err != nil {
		return &moduleBuild{err: err}
	}

	value := t.ctx.Encode(keyed)
	if value.Err() != nil {
		return &moduleBuild{err: value.Err()}
	}

	return &moduleBuild{objects: keyed, value: value}
}

// keyObjects returns the given objects keyed by their test identifier. The same
// kind can be served by more than one API group, so the identifier Timoni prints
// for an object is not unique on its own. The objects that would share one are
// keyed by their group-qualified identifier instead, which keeps every object
// addressable under exactly one key.
func keyObjects(objects []*unstructured.Unstructured) (map[string]any, error) {
	shared := make(map[string]int, len(objects))
	for _, object := range objects {
		shared[objectKey(object, false)]++
	}

	keyed := make(map[string]any, len(objects))
	for _, object := range objects {
		key := objectKey(object, shared[objectKey(object, false)] > 1)
		if _, ok := keyed[key]; ok {
			return nil, fmt.Errorf("the module renders more than one object identified by %s", key)
		}
		keyed[key] = object.Object
	}

	return keyed, nil
}

// checkCaseFields reports the fields a case declares that are not part of the
// test case schema. Cases are open structs, so without this check a misspelled
// field would be ignored and the case would run with the defaults it was
// written to override.
func checkCaseFields(value cue.Value) error {
	iter, err := value.Fields()
	if err != nil {
		return fmt.Errorf("reading the case fields failed: %w", err)
	}

	var unknown []string
	for iter.Next() {
		name := iter.Selector().Unquoted()
		if !slices.Contains(caseFields, apiv1.Selector(name)) {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) == 0 {
		return nil
	}

	known := make([]string, 0, len(caseFields))
	for _, field := range caseFields {
		known = append(known, field.String())
	}

	sort.Strings(unknown)
	return fmt.Errorf("unknown field(s) %s, a test case may declare %s",
		strings.Join(unknown, ", "), strings.Join(known, ", "))
}

// checkExpectedFields reports the fields a case expects under an object that
// the module does not render. The rendered objects are open structs, so an
// expectation naming a field that does not exist would be added to the object
// instead of compared against it, and the case would hold without checking it.
func checkExpectedFields(value, rendered cue.Value) error {
	objects := value.LookupPath(cue.ParsePath(apiv1.TestObjectsSelector.String()))
	if !objects.Exists() {
		return nil
	}

	var missing []string
	if err := collectMissingFields(objects, rendered, nil, &missing); err != nil {
		return fmt.Errorf("reading the expected objects failed: %w", err)
	}
	if len(missing) == 0 {
		return nil
	}

	sort.Strings(missing)
	return fmt.Errorf("expected field(s) %s not rendered by the module", strings.Join(missing, ", "))
}

// collectMissingFields walks an expectation alongside the value it is about to
// be unified with, and records the paths that value does not hold. The walk
// stops where the two differ in kind or where the list is shorter than
// expected, so that a mismatch is left to be reported as a conflict, which
// names both sides.
func collectMissingFields(expected, actual cue.Value, prefix []cue.Selector, missing *[]string) error {
	switch expected.IncompleteKind() {
	case cue.StructKind:
		if actual.IncompleteKind() != cue.StructKind {
			return nil
		}

		iter, err := expected.Fields()
		if err != nil {
			return err
		}

		for iter.Next() {
			selector := iter.Selector()
			path := append(slices.Clone(prefix), selector)

			field := actual.LookupPath(cue.MakePath(selector))
			if !field.Exists() {
				*missing = append(*missing, cue.MakePath(path...).String())
				continue
			}

			if err := collectMissingFields(iter.Value(), field, path, missing); err != nil {
				return err
			}
		}
	case cue.ListKind:
		if actual.IncompleteKind() != cue.ListKind {
			return nil
		}

		iter, err := expected.List()
		if err != nil {
			return err
		}

		for index := 0; iter.Next(); index++ {
			selector := cue.Index(index)
			path := append(slices.Clone(prefix), selector)

			element := actual.LookupPath(cue.MakePath(selector))
			if !element.Exists() {
				return nil
			}

			if err := collectMissingFields(iter.Value(), element, path, missing); err != nil {
				return err
			}
		}
	}

	return nil
}

// checkExpectedObjectsExist reports the object keys a case expects that the
// module did not render. Without this check an expectation naming an object
// that does not exist would unify with nothing and silently hold.
func checkExpectedObjectsExist(value cue.Value, rendered map[string]any) error {
	objects := value.LookupPath(cue.ParsePath(apiv1.TestObjectsSelector.String()))
	if !objects.Exists() {
		return nil
	}

	iter, err := objects.Fields()
	if err != nil {
		return fmt.Errorf("lookup %s failed: %w", apiv1.TestObjectsSelector, err)
	}

	var missing []string
	for iter.Next() {
		key := iter.Selector().Unquoted()
		if _, ok := rendered[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	sort.Strings(missing)
	available := make([]string, 0, len(rendered))
	for key := range rendered {
		available = append(available, key)
	}
	sort.Strings(available)

	return fmt.Errorf("expected object(s) %s not rendered, the module renders %s",
		strings.Join(missing, ", "), strings.Join(available, ", "))
}

// checkObjects validates the case's object expectations against the rendered
// objects they were unified with.
func checkObjects(filled cue.Value) error {
	objects := filled.LookupPath(cue.ParsePath(apiv1.TestObjectsSelector.String()))
	if !objects.Exists() {
		return nil
	}

	return objects.Validate(cue.Concrete(true), cue.All())
}

// checkAssertions requires every field declared under the case's 'assert'
// struct to evaluate to true.
func checkAssertions(filled cue.Value) error {
	assertions := filled.LookupPath(cue.ParsePath(apiv1.TestAssertSelector.String()))
	if !assertions.Exists() {
		return nil
	}

	iter, err := assertions.Fields()
	if err != nil {
		return fmt.Errorf("lookup %s failed: %w", apiv1.TestAssertSelector, err)
	}

	for iter.Next() {
		name := iter.Selector().Unquoted()
		holds, err := iter.Value().Bool()
		if err != nil {
			return fmt.Errorf("assertion %q could not be evaluated: %w", name, err)
		}
		if !holds {
			return fmt.Errorf("assertion %q does not hold", name)
		}
	}

	return nil
}

// stringField returns the string at the given selector, or the fallback when
// the field is absent. A field that is present but is not a concrete string is
// an error: falling back would build the case with a configuration other than
// the one its author asked for.
func stringField(value cue.Value, selector apiv1.Selector, fallback string) (string, error) {
	field := value.LookupPath(cue.ParsePath(selector.String()))
	if !field.Exists() {
		return fallback, nil
	}

	str, err := field.String()
	if err != nil {
		return "", fmt.Errorf("invalid %s: %w", selector, err)
	}

	return str, nil
}
