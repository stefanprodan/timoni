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
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/encoding/json"
	"cuelang.org/go/encoding/yaml"
	"github.com/Masterminds/semver/v3"
	"github.com/google/go-containerregistry/pkg/crane"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/oci"
)

const (
	// UpdateLevelNone leaves module references without an update attribute untouched.
	UpdateLevelNone string = "none"

	// UpdateLevelPatch updates module references without an update attribute
	// to the newest patch version of the current minor version.
	UpdateLevelPatch string = "patch"

	// UpdateLevelMinor updates module references without an update attribute
	// to the newest minor version of the current major version.
	UpdateLevelMinor string = "minor"

	// UpdateLevelMajor updates module references without an update attribute
	// to the newest version.
	UpdateLevelMajor string = "major"
)

// ModuleVersionLister lists the versions published in a module repository
// and resolves their digests.
type ModuleVersionLister interface {
	// ListVersions returns the module versions, newest first,
	// along with the latest tag when present.
	ListVersions(ctx context.Context, repository string) ([]string, error)

	// ResolveDigest returns the digest of the given module version.
	ResolveDigest(ctx context.Context, repository, version string) (string, error)
}

// OCIModuleVersionLister lists the module versions from an OCI repository.
type OCIModuleVersionLister struct {
	Opts []crane.Option
}

// ListVersions lists the semver tags of the module repository.
func (l *OCIModuleVersionLister) ListVersions(ctx context.Context, repository string) ([]string, error) {
	list, _, err := oci.ListModuleVersions(ctx, repository, oci.ListModuleOptions{}, l.Opts)
	if err != nil {
		return nil, err
	}
	versions := make([]string, 0, len(list))
	for _, ref := range list {
		versions = append(versions, ref.Version)
	}
	return versions, nil
}

// ResolveDigest resolves the digest of the module version tag.
func (l *OCIModuleVersionLister) ResolveDigest(_ context.Context, repository, version string) (string, error) {
	ref, err := oci.GetArtifactDigest(fmt.Sprintf("%s:%s", repository, version), l.Opts)
	if err != nil {
		return "", err
	}
	return ref.Digest, nil
}

// UpdatePolicy describes how the version of a module reference is selected.
type UpdatePolicy struct {
	// Policy is one of the apiv1.UpdatePolicy* values.
	Policy string

	// Constraint is the semver constraint of the semver policy.
	Constraint string
}

// UpdateChange records a module reference change for the instances
// whose version is defined by the same literal.
type UpdateChange struct {
	Instances   []string
	Repository  string
	FromSource  string
	ToSource    string
	FromVersion string
	ToVersion   string
	FromDigest  string
	ToDigest    string

	// Files are the paths of the bundle files holding the rewritten literals.
	Files []string

	versionLit *bundleLiteral
	sourceLits []*bundleLiteral
	digestLits []*bundleLiteral
}

// UpdateSkip records an instance excluded from the update and the reason.
type UpdateSkip struct {
	Instance   string
	Repository string
	Reason     string
}

// UpdatePlan holds the module reference changes computed for a bundle.
type UpdatePlan struct {
	Changes []*UpdateChange
	Skipped []*UpdateSkip
}

// bundleLiteral is a string literal found in a bundle file, along with
// the field declaring it when the literal is a field value.
type bundleLiteral struct {
	lit   *ast.BasicLit
	field *ast.Field
	file  *bundleFile
}

// bundleFile is a bundle definition loaded in the updater workspace,
// either an entry file passed to the updater or a package file
// of the CUE module imported by the entry files.
type bundleFile struct {
	// origin is the path of the file on disk.
	origin string

	// content is the file content read from disk when loaded.
	content []byte

	// syntax is the parsed file.
	syntax *ast.File

	// literals indexes the string literals of the file by byte offset,
	// nil for non-CUE files whose literals cannot be rewritten.
	literals map[int]*bundleLiteral
}

// updateTarget is the module reference of a bundle instance
// resolved to the literals defining it.
type updateTarget struct {
	instance   string
	repository string
	source     string
	version    string
	digest     string
	sourceLit  *bundleLiteral
	indexed    bool
	policies   []UpdatePolicy
	versionLit *bundleLiteral
	digestLit  *bundleLiteral
	skip       string

	// versionReason explains why the version cannot be rewritten
	// when versionLit is nil, and versionUnset marks the version
	// as the schema default.
	versionReason string
	versionUnset  bool
}

// updateGroup gathers the targets whose version is defined by the same literal.
type updateGroup struct {
	targets    []*updateTarget
	policies   []UpdatePolicy
	sourceLits []*bundleLiteral
	digestLits []*bundleLiteral
}

// BundleUpdater updates the module versions and digests referenced
// in bundle files according to the update policies declared with
// @timoni(update:...) attributes. The literals are rewritten both in
// the entry files and in the package files of the CUE module that
// the entry files import.
type BundleUpdater struct {
	ctx     *cue.Context
	files   []string
	root    string
	workdir string
	level   string

	// workspace maps the file paths served to the CUE loader to the
	// loaded bundle files: the entry files under their in-memory paths
	// listed in workspaceFiles, and the imported package files under
	// their on-disk paths listed in packageFiles.
	workspace      map[string]*bundleFile
	workspaceFiles []string
	packageFiles   []string
	value          cue.Value
	localIndex     *LocalModuleIndexLister
	localArtifacts *LocalModuleArtifactLister
}

// NewBundleUpdater creates a BundleUpdater for the given bundle files.
func NewBundleUpdater(ctx *cue.Context, files []string) *BundleUpdater {
	return &BundleUpdater{
		ctx:       ctx,
		files:     files,
		root:      filepath.Join(os.TempDir(), apiv1.FieldManager, "update"),
		level:     UpdateLevelNone,
		workspace: make(map[string]*bundleFile, len(files)),
	}
}

// SetWorkdir sets the directory from which the CUE loader discovers the
// cue.mod module root, enabling imports in the bundle definitions.
func (u *BundleUpdater) SetWorkdir(dir string) {
	u.workdir = dir
}

// SetLocalIndex enables bundle updates for local modules through an explicit
// index. Without an index, file:// module references remain excluded.
func (u *BundleUpdater) SetLocalIndex(index *LocalModuleIndexLister) {
	u.localIndex = index
}

// SetLocalArtifacts enables bundle updates from explicitly mapped local OCI
// archives and image layouts.
func (u *BundleUpdater) SetLocalArtifacts(artifacts *LocalModuleArtifactLister) {
	u.localArtifacts = artifacts
}

// SetLevel sets the update level applied to the module references
// that don't declare an update attribute. Must be one of the UpdateLevel* values.
func (u *BundleUpdater) SetLevel(level string) error {
	switch level {
	case UpdateLevelNone, UpdateLevelPatch, UpdateLevelMinor, UpdateLevelMajor:
		u.level = level
		return nil
	default:
		return fmt.Errorf("unknown update level %q, must be one of %s, %s, %s or %s",
			level, UpdateLevelNone, UpdateLevelPatch, UpdateLevelMinor, UpdateLevelMajor)
	}
}

// Load parses the bundle files and builds the bundle value.
// CUE files are kept as syntax trees so that their literals can be rewritten,
// YAML and JSON files are read-only and only contribute to the bundle value.
// The package files of the CUE module imported by the entry files are
// loaded as well, so that the module references they define can be updated.
func (u *BundleUpdater) Load() error {
	u.workspaceFiles = u.workspaceFiles[:0]
	u.packageFiles = u.packageFiles[:0]
	for i, file := range u.files {
		_, fn := filepath.Split(file)
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", fn, err)
		}

		wsFile := filepath.Join(u.root, fmt.Sprintf("%v.%s.cue", i, strings.TrimSuffix(fn, ".cue")))
		bf := &bundleFile{origin: file, content: content}
		switch ext := filepath.Ext(fn); ext {
		case ".cue":
			bf.syntax, err = parser.ParseFile(wsFile, content, parser.ParseComments)
			if err != nil {
				return fmt.Errorf("failed to parse %s: %w", fn, err)
			}
			bf.literals = indexLiterals(bf)
		case ".yaml", ".yml":
			bf.syntax, err = yaml.Extract(wsFile, content)
			if err != nil {
				return fmt.Errorf("failed to parse %s: %w", fn, err)
			}
		case ".json":
			expr, err := json.Extract(wsFile, content)
			if err != nil {
				return fmt.Errorf("failed to parse %s: %w", fn, err)
			}
			bf.syntax = &ast.File{Filename: wsFile, Decls: []ast.Decl{&ast.EmbedDecl{Expr: expr}}}
		default:
			return fmt.Errorf("unsupported file extension: %s", ext)
		}

		u.workspace[wsFile] = bf
		u.workspaceFiles = append(u.workspaceFiles, wsFile)
	}

	inst, err := u.build()
	if err != nil {
		return err
	}

	for _, path := range modulePackageFiles(inst) {
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}
		bf := &bundleFile{origin: path, content: content}
		bf.syntax, err = parser.ParseFile(path, content, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", path, err)
		}
		bf.literals = indexLiterals(bf)
		u.workspace[path] = bf
		u.packageFiles = append(u.packageFiles, path)
	}

	return nil
}

// modulePackageFiles returns the paths of the CUE files of the packages
// imported by the instance, directly or transitively, that are located
// inside the CUE module of the instance. Files of the dependencies vendored
// under cue.mod or fetched from a registry are excluded, as are files that
// resolve through symlinks to a location outside the module.
func modulePackageFiles(inst *build.Instance) []string {
	if inst.Root == "" {
		return nil
	}
	root, err := filepath.EvalSymlinks(inst.Root)
	if err != nil {
		return nil
	}
	cueMod := filepath.Join(root, "cue.mod")
	inside := func(path string) bool {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return false
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return false
		}
		return resolved != cueMod && !strings.HasPrefix(resolved, cueMod+string(filepath.Separator))
	}

	var files []string
	seen := make(map[string]bool)
	var walk func(*build.Instance)
	walk = func(in *build.Instance) {
		for _, imp := range in.Imports {
			if seen[imp.ImportPath] {
				continue
			}
			seen[imp.ImportPath] = true
			if imp.Root != inst.Root || !inside(imp.Dir) {
				continue
			}
			for _, f := range imp.Files {
				if filepath.Ext(f.Filename) == ".cue" && inside(f.Filename) && !slices.Contains(files, f.Filename) {
					files = append(files, f.Filename)
				}
			}
			walk(imp)
		}
	}
	walk(inst)
	return files
}

// build evaluates the bundle files along with the bundle schema and
// returns the loaded instance. Only the module references must be
// concrete, the instance values may hold runtime attributes that are
// left unset. The package files already loaded in the workspace are
// served from their syntax trees so that rewritten literals take effect.
func (u *BundleUpdater) build() (*build.Instance, error) {
	overlay := make(map[string]load.Source, len(u.workspaceFiles)+len(u.packageFiles)+1)
	for _, wsFile := range u.workspaceFiles {
		overlay[wsFile] = load.FromFile(u.workspace[wsFile].syntax)
	}
	for _, path := range u.packageFiles {
		overlay[path] = load.FromFile(u.workspace[path].syntax)
	}

	schemaFile := filepath.Join(u.root, fmt.Sprintf("%v.schema.cue", len(u.workspaceFiles)))
	overlay[schemaFile] = load.FromBytes([]byte(apiv1.BundleSchema))

	cfg := &load.Config{
		Package:   "_",
		DataFiles: true,
		Overlay:   overlay,
		Dir:       u.workdir,
	}

	ix := load.Instances(append(slices.Clone(u.workspaceFiles), schemaFile), cfg)
	if len(ix) == 0 {
		return nil, fmt.Errorf("no instances found")
	}
	if ix[0].Err != nil {
		return nil, fmt.Errorf("instance error: %w", ix[0].Err)
	}

	v := u.ctx.BuildInstance(ix[0])
	if v.Err() != nil {
		return nil, v.Err()
	}

	if err := v.Validate(); err != nil {
		return nil, err
	}

	u.value = v
	return ix[0], nil
}

// indexLiterals returns the string literals declared as field values
// or let bindings in the file, indexed by their byte offset.
func indexLiterals(bf *bundleFile) map[int]*bundleLiteral {
	index := make(map[int]*bundleLiteral)
	ast.Walk(bf.syntax, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Field:
			if lit, ok := x.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				index[lit.Pos().Offset()] = &bundleLiteral{lit: lit, field: x, file: bf}
			}
		case *ast.LetClause:
			if lit, ok := x.Expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				index[lit.Pos().Offset()] = &bundleLiteral{lit: lit, file: bf}
			}
		}
		return true
	}, nil)
	return index
}

// lookupLiteral returns the literal defining the given concrete string value.
// The result is nil when the value is not defined by a rewritable literal,
// e.g. it comes from the bundle schema, a non-CUE file or an interpolation.
func (u *BundleUpdater) lookupLiteral(v cue.Value) *bundleLiteral {
	if _, ok := v.Source().(*ast.BasicLit); !ok {
		return nil
	}
	pos := v.Pos()
	bf, ok := u.workspace[pos.Filename()]
	if !ok || bf.literals == nil {
		return nil
	}
	return bf.literals[pos.Offset()]
}

// literalSkipReason explains why the given module reference field value
// is not defined by a rewritable literal, e.g. the module version is an
// interpolation, or is set in a YAML or JSON file or in a CUE file that
// is not part of the bundle CUE module.
func (u *BundleUpdater) literalSkipReason(v cue.Value, field string) string {
	if _, ok := v.Source().(*ast.BasicLit); !ok {
		return fmt.Sprintf("module %s is not a string literal", field)
	}
	pos := v.Pos()
	if bf, ok := u.workspace[pos.Filename()]; ok && bf.literals == nil {
		return fmt.Sprintf("module %s is defined in %s which is not a CUE file", field, filepath.Base(bf.origin))
	}
	return fmt.Sprintf("module %s is defined in %s outside the bundle CUE module", field, filepath.Base(pos.Filename()))
}

// definitions returns the number of distinct places in the bundle files
// where the given value is defined.
func (u *BundleUpdater) definitions(v cue.Value) int {
	op, args := v.Expr()
	if op != cue.AndOp {
		return 1
	}
	positions := make(map[string]struct{})
	for _, arg := range args {
		pos := arg.Pos()
		if _, ok := u.workspace[pos.Filename()]; ok {
			positions[fmt.Sprintf("%s:%d", pos.Filename(), pos.Offset())] = struct{}{}
		}
	}
	return len(positions)
}

// isRuntimeManaged returns true if the given field value or the field
// defining its literal is injected from a runtime value.
func isRuntimeManaged(v cue.Value, lit *bundleLiteral) bool {
	for _, a := range v.Attributes(cue.FieldAttr) {
		if apiv1.IsRuntimeAttribute(a.Name(), a.Contents()) {
			return true
		}
	}
	if lit != nil && lit.field != nil {
		for _, a := range lit.field.Attrs {
			if apiv1.IsRuntimeAttribute(a.Split()) {
				return true
			}
		}
	}
	return false
}

// policiesOf returns the update policies declared on the given field value
// and on the field defining its literal, if any.
func policiesOf(v cue.Value, lit *bundleLiteral) ([]UpdatePolicy, error) {
	var policies []UpdatePolicy
	add := func(key, body string) error {
		if key != apiv1.FieldManager {
			return nil
		}
		if kind := strings.SplitN(body, apiv1.RuntimeDelimiter, 2)[0]; kind != apiv1.UpdateKind {
			return nil
		}
		attr, err := apiv1.NewUpdateAttribute(key, body)
		if err != nil {
			return fmt.Errorf("attribute '@%s(%s)': %w", key, body, err)
		}
		policy := UpdatePolicy{Policy: attr.Policy, Constraint: attr.Constraint}
		if policy.Policy == apiv1.UpdatePolicySemver {
			if _, err := semver.NewConstraint(policy.Constraint); err != nil {
				return fmt.Errorf("attribute '@%s(%s)': invalid semver constraint: %w", key, body, err)
			}
		}
		if !slices.Contains(policies, policy) {
			policies = append(policies, policy)
		}
		return nil
	}

	for _, a := range v.Attributes(cue.FieldAttr) {
		if err := add(a.Name(), a.Contents()); err != nil {
			return nil, err
		}
	}
	if lit != nil && lit.field != nil {
		for _, a := range lit.field.Attrs {
			if err := add(a.Split()); err != nil {
				return nil, err
			}
		}
	}
	return policies, nil
}

// resolveTargetSource resolves the module source of a bundle instance and
// reports whether the target can be considered for updating.
func (u *BundleUpdater) resolveTargetSource(t *updateTarget, vURL cue.Value) (bool, error) {
	if strings.HasPrefix(t.repository, apiv1.LocalPrefix) {
		if u.localIndex == nil && u.localArtifacts == nil {
			t.skip = "module is not an OCI artifact"
			return false, nil
		}
		baseDir := filepath.Dir(vURL.Pos().Filename())
		if t.sourceLit != nil {
			baseDir = filepath.Dir(t.sourceLit.file.origin)
		}
		var identity string
		var ok bool
		var resolveErr error
		if u.localIndex != nil {
			identity, ok, resolveErr = u.localIndex.ResolveIdentity(t.repository, baseDir)
		}
		if u.localArtifacts != nil {
			artifactIdentity, artifactOK, artifactErr := u.localArtifacts.ResolveIdentity(t.repository, baseDir)
			if artifactErr != nil {
				return false, fmt.Errorf("instance %s: %w", t.instance, artifactErr)
			}
			if ok && artifactOK && identity != artifactIdentity {
				return false, fmt.Errorf("instance %s: local source maps to conflicting module identities %s and %s", t.instance, identity, artifactIdentity)
			}
			if artifactOK {
				identity, ok = artifactIdentity, true
			}
		}
		if resolveErr != nil {
			return false, fmt.Errorf("instance %s: %w", t.instance, resolveErr)
		}
		if !ok {
			t.skip = "module source is not in the local index"
			return false, nil
		}
		t.repository = identity
		t.indexed = true
		if t.sourceLit == nil {
			t.skip = "module url is not a rewritable CUE literal"
			return false, nil
		}
		return true, nil
	}
	if !strings.HasPrefix(t.repository, apiv1.ArtifactPrefix) {
		t.skip = "module is not an OCI artifact"
		return false, nil
	}
	if u.localIndex == nil && u.localArtifacts == nil {
		return true, nil
	}
	indexOK := false
	if u.localIndex != nil {
		_, indexOK = u.localIndex.entries[t.repository]
	}
	artifactOK := u.localArtifacts != nil && u.localArtifacts.HasRepository(t.repository)
	if indexOK || artifactOK {
		t.indexed = true
		if t.sourceLit == nil {
			t.skip = "module url is not a rewritable CUE literal"
			return false, nil
		}
	}
	return true, nil
}

// targets resolves the module reference of each bundle instance
// to the literals defining its version and digest.
func (u *BundleUpdater) targets() ([]*updateTarget, error) {
	instances := u.value.LookupPath(cue.ParsePath(apiv1.BundleInstancesSelector.String()))
	if instances.Err() != nil {
		return nil, fmt.Errorf("lookup %s failed: %w", apiv1.BundleInstancesSelector.String(), instances.Err())
	}

	iter, err := instances.Fields()
	if err != nil {
		return nil, err
	}

	var targets []*updateTarget
	for iter.Next() {
		t := &updateTarget{instance: iter.Selector().Unquoted()}
		targets = append(targets, t)
		expr := iter.Value()

		vURL := expr.LookupPath(cue.ParsePath(apiv1.BundleModuleURLSelector.String()))
		if isRuntimeManaged(vURL, u.lookupLiteral(vURL)) {
			t.skip = "module url is set from a runtime value"
			continue
		}
		t.repository, err = vURL.String()
		if err != nil {
			t.skip = "module url is not concrete"
			continue
		}
		t.source = t.repository
		t.sourceLit = u.lookupLiteral(vURL)
		canUpdateSource, err := u.resolveTargetSource(t, vURL)
		if err != nil {
			return nil, err
		}
		if !canUpdateSource {
			continue
		}

		vVersion := expr.LookupPath(cue.ParsePath(apiv1.BundleModuleVersionSelector.String()))
		t.versionLit = u.lookupLiteral(vVersion)
		if isRuntimeManaged(vVersion, t.versionLit) {
			t.skip = "module version is set from a runtime value"
			continue
		}
		t.version, err = vVersion.String()
		if err != nil {
			t.skip = "module version is not concrete"
			continue
		}
		t.policies, err = policiesOf(vVersion, t.versionLit)
		if err != nil {
			return nil, fmt.Errorf("instance %s: %w", t.instance, err)
		}

		vDigest := expr.LookupPath(cue.ParsePath(apiv1.BundleModuleDigestSelector.String()))
		if vDigest.Exists() {
			t.digestLit = u.lookupLiteral(vDigest)
			if isRuntimeManaged(vDigest, t.digestLit) {
				t.skip = "module digest is set from a runtime value"
				continue
			}
			t.digest, err = vDigest.String()
			if err != nil {
				t.skip = "module digest is not concrete"
				continue
			}
			if t.digestLit == nil {
				t.skip = u.literalSkipReason(vDigest, "digest")
				continue
			}
			if u.definitions(vDigest) > 1 {
				t.skip = "module digest is defined in multiple places"
				continue
			}
			digestPolicies, err := policiesOf(vDigest, t.digestLit)
			if err != nil {
				return nil, fmt.Errorf("instance %s: %w", t.instance, err)
			}
			for _, p := range digestPolicies {
				if !slices.Contains(t.policies, p) {
					t.policies = append(t.policies, p)
				}
			}
		}
		if t.indexed && t.digestLit == nil {
			t.skip = "indexed module requires a digest literal"
			continue
		}

		if t.versionLit == nil {
			if _, isField := vVersion.Source().(*ast.Field); isField {
				// The version is the schema default.
				t.versionReason = "module version is not set"
				t.versionUnset = true
			} else {
				t.versionReason = u.literalSkipReason(vVersion, "version")
			}
			// Only the digest can be updated.
			if t.digestLit == nil {
				t.skip = t.versionReason
			}
			continue
		}
		if u.definitions(vVersion) > 1 {
			t.skip = "module version is defined in multiple places"
		}
	}

	return targets, nil
}

// groupTargets gathers the targets whose version is defined by the same
// literal, in the order of the instances. Targets sharing a version literal
// must reference the same module repository, and a digest literal cannot
// be shared by targets with different version literals.
func groupTargets(targets []*updateTarget) ([]*updateGroup, error) {
	groups := make(map[*ast.BasicLit]*updateGroup)
	digestOwners := make(map[*ast.BasicLit]*updateGroup)
	var order []*updateGroup
	for _, t := range targets {
		key := t.digestLit
		if t.versionLit != nil {
			key = t.versionLit
		}
		g, ok := groups[key.lit]
		if !ok {
			g = &updateGroup{}
			groups[key.lit] = g
			order = append(order, g)
		}
		if len(g.targets) > 0 && g.targets[0].repository != t.repository {
			return nil, fmt.Errorf("instances %s and %s share the module version but reference different modules %s and %s",
				g.targets[0].instance, t.instance, g.targets[0].repository, t.repository)
		}
		g.targets = append(g.targets, t)
		if t.sourceLit != nil && !slices.Contains(g.sourceLits, t.sourceLit) {
			g.sourceLits = append(g.sourceLits, t.sourceLit)
		}
		for _, p := range t.policies {
			if !slices.Contains(g.policies, p) {
				g.policies = append(g.policies, p)
			}
		}
		if t.digestLit != nil {
			if owner, ok := digestOwners[t.digestLit.lit]; ok && owner != g {
				return nil, fmt.Errorf("instances %s and %s share the module digest but not the module version",
					owner.targets[0].instance, t.instance)
			}
			digestOwners[t.digestLit.lit] = g
			if !slices.Contains(g.digestLits, t.digestLit) {
				g.digestLits = append(g.digestLits, t.digestLit)
			}
		}
	}
	return order, nil
}

// names returns the instance names of the group.
func (g *updateGroup) names() []string {
	names := make([]string, 0, len(g.targets))
	for _, t := range g.targets {
		names = append(names, t.instance)
	}
	return names
}

// current returns the module reference shared by the group, with the digest
// of the first target pinning one, and whether the pinned digests differ
// between the targets.
func (g *updateGroup) current() (apiv1.ModuleReference, bool) {
	first := g.targets[0]
	ref := apiv1.ModuleReference{Repository: first.repository, Version: first.version}
	drift := false
	for _, t := range g.targets {
		if t.digestLit == nil {
			continue
		}
		if ref.Digest == "" {
			ref.Digest = t.digest
		}
		if t.digest != ref.Digest {
			drift = true
		}
	}
	return ref, drift
}

// policy returns the update policy of the group from the attributes declared
// by its targets, falling back to the update level. When the level cannot
// apply to the group, the returned reason explains why it is skipped.
func (g *updateGroup) policy(level string) (UpdatePolicy, string, error) {
	first := g.targets[0]
	switch {
	case len(g.policies) > 1:
		return UpdatePolicy{}, "", fmt.Errorf("instances %s share the module version but declare different update policies",
			strings.Join(g.names(), ", "))
	case len(g.policies) == 1:
		policy := g.policies[0]
		if first.versionLit == nil && policy.Policy == apiv1.UpdatePolicySemver {
			return UpdatePolicy{}, "", fmt.Errorf("instances %s: %s, the %s policy requires a version literal",
				strings.Join(g.names(), ", "), first.versionReason, policy.Policy)
		}
		return policy, "", nil
	case first.versionLit == nil:
		switch {
		case level == UpdateLevelNone:
			return UpdatePolicy{Policy: apiv1.UpdatePolicyNone}, "", nil
		case first.versionUnset:
			// The version is unset and tracks the latest tag, the digest follows it.
			return UpdatePolicy{Policy: apiv1.UpdatePolicyDigest}, "", nil
		default:
			return UpdatePolicy{}, first.versionReason, nil
		}
	default:
		policy, reason := policyForLevel(level, first.version)
		return policy, reason, nil
	}
}

func verifyIndexedSource(ctx context.Context, resolver ModuleSourceLister, first *updateTarget, names []string, version string) error {
	if !first.indexed || resolver == nil {
		return nil
	}
	expectedSource, err := resolver.ResolveSource(ctx, first.repository, version)
	if err != nil {
		return fmt.Errorf("instances %s: verifying local source for %s version %s failed: %w", strings.Join(names, ", "), first.repository, version, err)
	}
	if !strings.HasPrefix(first.source, apiv1.LocalPrefix) {
		return nil
	}
	currentSource, resolveErr := canonicalLocalSource(first.source, filepath.Dir(first.sourceLit.file.origin))
	if resolveErr != nil || currentSource != expectedSource {
		return fmt.Errorf("instances %s: local source does not match the indexed %s version %s", strings.Join(names, ", "), first.repository, version)
	}
	return nil
}

func resolveIndexedSource(ctx context.Context, resolver ModuleSourceLister, first *updateTarget, version string) (string, error) {
	if !first.indexed || resolver == nil {
		return "", nil
	}
	source, err := resolver.ResolveSource(ctx, first.repository, version)
	if err != nil {
		return "", fmt.Errorf("resolving local source for %s version %s failed: %w", first.repository, version, err)
	}
	if strings.HasPrefix(first.source, apiv1.LocalPrefix) {
		baseDir := filepath.Dir(first.sourceLit.file.origin)
		currentSource, resolveErr := canonicalLocalSource(first.source, baseDir)
		if resolveErr == nil && currentSource == source {
			return "", nil
		}
		if source == "" {
			return first.repository, nil
		}
	}
	return source, nil
}

// Plan computes the module reference changes for the bundle instances
// by listing the module versions with the given lister and selecting
// the version according to the update policy of each instance. The digest
// of the selected version is resolved only for the instances pinning one.
// Instances sharing the same version literal are updated together and
// must reference the same module repository with the same policy.
// Errors listing or selecting the versions are collected for all the
// instances and returned along with the changes of the other instances.
func (u *BundleUpdater) Plan(ctx context.Context, lister ModuleVersionLister) (*UpdatePlan, error) {
	targets, err := u.targets()
	if err != nil {
		return nil, err
	}

	plan := &UpdatePlan{}
	var active []*updateTarget
	for _, t := range targets {
		if t.skip != "" {
			plan.Skipped = append(plan.Skipped, &UpdateSkip{Instance: t.instance, Repository: t.repository, Reason: t.skip})
			continue
		}
		active = append(active, t)
	}

	groups, err := groupTargets(active)
	if err != nil {
		return nil, err
	}

	var errs []error
	versions := make(map[string][]string)
	for _, g := range groups {
		first := g.targets[0]
		names := g.names()
		skip := func(reason string) {
			for _, t := range g.targets {
				plan.Skipped = append(plan.Skipped, &UpdateSkip{Instance: t.instance, Repository: t.repository, Reason: reason})
			}
		}

		policy, reason, err := g.policy(u.level)
		if err != nil {
			return nil, err
		}
		switch {
		case reason != "":
			skip(reason)
			continue
		case policy.Policy == apiv1.UpdatePolicyNone:
			skip("update policy is none")
			continue
		case policy.Policy == apiv1.UpdatePolicyDigest && len(g.digestLits) == 0:
			skip("update policy is digest but the module digest is not set")
			continue
		}

		list, ok := versions[first.repository]
		if !ok {
			list, err = lister.ListVersions(ctx, first.repository)
			if err != nil {
				errs = append(errs, fmt.Errorf("instances %s: listing versions of %s failed: %w",
					strings.Join(names, ", "), first.repository, err))
				continue
			}
			versions[first.repository] = list
		}

		current, digestDrift := g.current()
		resolver, _ := lister.(ModuleSourceLister)
		if err := verifyIndexedSource(ctx, resolver, first, names, current.Version); err != nil {
			errs = append(errs, err)
			continue
		}
		next, err := selectVersion(policy, current, list)
		if err != nil {
			errs = append(errs, fmt.Errorf("instances %s: %w", strings.Join(names, ", "), err))
			continue
		}

		if len(g.digestLits) > 0 {
			next.Digest, err = lister.ResolveDigest(ctx, first.repository, next.Version)
			if err != nil {
				errs = append(errs, fmt.Errorf("instances %s: resolving the digest of %s version %s failed: %w",
					strings.Join(names, ", "), first.repository, next.Version, err))
				continue
			}
			if next.Digest != current.Digest {
				digestDrift = true
			}
		}
		toSource, err := resolveIndexedSource(ctx, resolver, first, next.Version)
		if err != nil {
			errs = append(errs, fmt.Errorf("instances %s: %w", strings.Join(names, ", "), err))
			continue
		}
		if next.Version == current.Version && !digestDrift && toSource == "" {
			skip("up to date")
			continue
		}

		change := &UpdateChange{
			Instances:   names,
			Repository:  first.repository,
			FromSource:  first.source,
			ToSource:    toSource,
			FromVersion: current.Version,
			ToVersion:   next.Version,
			versionLit:  first.versionLit,
			sourceLits:  g.sourceLits,
			digestLits:  g.digestLits,
		}
		if first.versionLit != nil {
			change.Files = append(change.Files, first.versionLit.file.origin)
		}
		if first.sourceLit != nil && toSource != "" && !slices.Contains(change.Files, first.sourceLit.file.origin) {
			change.Files = append(change.Files, first.sourceLit.file.origin)
		}
		for _, lit := range g.digestLits {
			if !slices.Contains(change.Files, lit.file.origin) {
				change.Files = append(change.Files, lit.file.origin)
			}
		}
		if len(g.digestLits) > 0 {
			change.FromDigest = current.Digest
			change.ToDigest = next.Digest
		}
		plan.Changes = append(plan.Changes, change)
	}

	return plan, errors.Join(errs...)
}

// Apply rewrites the version and digest literals with the planned changes
// and rebuilds the bundle to verify that the result is a valid bundle.
func (u *BundleUpdater) Apply(plan *UpdatePlan) error {
	for _, change := range plan.Changes {
		if change.ToSource != "" {
			for _, lit := range change.sourceLits {
				lit.lit.Value = literal.String.Quote(change.ToSource)
			}
		}
		if change.versionLit != nil {
			change.versionLit.lit.Value = literal.String.Quote(change.ToVersion)
		}
		for _, lit := range change.digestLits {
			lit.lit.Value = literal.String.Quote(change.ToDigest)
		}
	}

	if _, err := u.build(); err != nil {
		return fmt.Errorf("the updated bundle is invalid: %w", err)
	}
	return nil
}

// Value returns the bundle value built from the current state of the files.
func (u *BundleUpdater) Value() cue.Value {
	return u.value
}

// Source returns the content of the given entry or package file
// as read from disk when the bundle was loaded.
func (u *BundleUpdater) Source(file string) ([]byte, error) {
	bf, err := u.lookupFile(file)
	if err != nil {
		return nil, err
	}
	return bf.content, nil
}

// Format returns the formatted content of the given entry or package file
// from its current syntax tree.
func (u *BundleUpdater) Format(file string) ([]byte, error) {
	bf, err := u.lookupFile(file)
	if err != nil {
		return nil, err
	}
	if bf.literals == nil {
		return nil, fmt.Errorf("%s is not a CUE file", file)
	}
	return format.Node(bf.syntax)
}

// lookupFile returns the loaded entry or package file with the given path.
func (u *BundleUpdater) lookupFile(file string) (*bundleFile, error) {
	for _, path := range slices.Concat(u.workspaceFiles, u.packageFiles) {
		if bf := u.workspace[path]; bf.origin == file {
			return bf, nil
		}
	}
	return nil, fmt.Errorf("%s is not a bundle file", file)
}

// policyForLevel returns the update policy for a module reference without
// an update attribute, derived from the level and the current version.
// When the level cannot apply to the current version, the returned reason
// explains why the reference is skipped.
func policyForLevel(level, current string) (UpdatePolicy, string) {
	if level == UpdateLevelNone {
		return UpdatePolicy{Policy: apiv1.UpdatePolicyNone}, ""
	}

	v, err := semver.StrictNewVersion(current)
	if err != nil {
		if level == UpdateLevelMajor {
			return UpdatePolicy{Policy: apiv1.UpdatePolicySemver, Constraint: "*"}, ""
		}
		return UpdatePolicy{}, fmt.Sprintf("module version %q is not a semantic version, the %s level cannot apply", current, level)
	}

	var constraint string
	switch level {
	case UpdateLevelPatch:
		constraint = fmt.Sprintf(">=%s <%d.%d.0", v.String(), v.Major(), v.Minor()+1)
	case UpdateLevelMinor:
		constraint = fmt.Sprintf(">=%s <%d.0.0", v.String(), v.Major()+1)
	default:
		constraint = ">=" + v.String()
	}
	return UpdatePolicy{Policy: apiv1.UpdatePolicySemver, Constraint: constraint}, ""
}

// selectVersion returns the module reference with the version selected by
// the policy from the published versions, without a digest. The version is
// never downgraded: when no published version matching the constraint is
// newer than the current one, the current version is kept. Pre-release
// versions are candidates only when the current version is itself a
// pre-release, and they are matched against the constraint without their
// pre-release suffix.
func selectVersion(policy UpdatePolicy, current apiv1.ModuleReference, list []string) (apiv1.ModuleReference, error) {
	keep := apiv1.ModuleReference{Repository: current.Repository, Version: current.Version}
	refresh := func() (apiv1.ModuleReference, error) {
		if !slices.Contains(list, current.Version) {
			return keep, fmt.Errorf("version %s not found in %s", current.Version, current.Repository)
		}
		return keep, nil
	}

	switch policy.Policy {
	case apiv1.UpdatePolicyDigest:
		return refresh()
	case apiv1.UpdatePolicySemver:
		constraint, err := semver.NewConstraint(policy.Constraint)
		if err != nil {
			return keep, fmt.Errorf("invalid semver constraint %q: %w", policy.Constraint, err)
		}

		currentVer, _ := semver.StrictNewVersion(current.Version)
		includePre := currentVer != nil && currentVer.Prerelease() != ""
		var candidate string
		var candidateVer *semver.Version
		for _, version := range list {
			ver, err := semver.StrictNewVersion(version)
			if err != nil {
				continue
			}
			core := *ver
			if ver.Prerelease() != "" {
				if !includePre {
					continue
				}
				core, _ = ver.SetPrerelease("")
			}
			if !constraint.Check(&core) {
				continue
			}
			if candidateVer == nil || ver.GreaterThan(candidateVer) {
				candidate = version
				candidateVer = ver
			}
		}

		if candidateVer == nil {
			return keep, fmt.Errorf("no version of %s matches the constraint %q", current.Repository, policy.Constraint)
		}
		if currentVer != nil && !candidateVer.GreaterThan(currentVer) {
			return refresh()
		}
		return apiv1.ModuleReference{Repository: current.Repository, Version: candidate}, nil
	default:
		return keep, nil
	}
}
