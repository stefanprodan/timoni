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
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/encoding/json"
	"cuelang.org/go/encoding/yaml"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// BundleBuilder compiles CUE definitions to Go Bundle objects.
type BundleBuilder struct {
	ctx               *cue.Context
	files             []string
	workspaceFiles    map[string][]string
	mapSourceToOrigin map[string]string
	injector          *RuntimeInjector
}

type Bundle struct {
	Name      string
	Instances []*BundleInstance
}

type BundleInstance struct {
	Bundle    string
	Cluster   string
	Name      string
	Namespace string
	Module    apiv1.ModuleReference
	Values    cue.Value
}

// NewBundleBuilder creates a BundleBuilder for the given module and package.
func NewBundleBuilder(ctx *cue.Context, files []string) *BundleBuilder {
	if ctx == nil {
		ctx = cuecontext.New()
	}
	b := &BundleBuilder{
		ctx:               ctx,
		files:             files,
		workspaceFiles:    make(map[string][]string),
		mapSourceToOrigin: make(map[string]string, len(files)),
		injector:          NewRuntimeInjector(ctx),
	}
	return b
}

// InitWorkspace copies the bundle definitions to the specified workspace,
// sets the bundle schema, and then it injects the runtime values based on @timoni() attributes.
// A workspace must be initialised before calling Build.
func (b *BundleBuilder) InitWorkspace(workspace string, runtimeValues map[string]string) error {
	var files []string
	for i, file := range b.files {
		_, fn := filepath.Split(file)
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", fn, err)
		}

		var parsefn func(string, []byte) (ast.Node, error)
		switch ext := filepath.Ext(fn); ext {
		case ".yaml", ".yml":
			parsefn = func(filename string, src []byte) (ast.Node, error) { return yaml.Extract(filename, src) }
		case ".json":
			parsefn = func(filename string, src []byte) (ast.Node, error) { return json.Extract(filename, src) }
		case ".cue":
			parsefn = func(filename string, src []byte) (ast.Node, error) {
				return parser.ParseFile(filename, src, parser.ParseComments)
			}
		default:
			parsefn = func(filename string, src []byte) (ast.Node, error) {
				return nil, fmt.Errorf("unsupported file extension: %s", ext)
			}
		}

		node, err := parsefn(fn, content)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", fn, err)
		}

		data, err := b.injector.Inject(node, runtimeValues)
		if err != nil {
			return fmt.Errorf("failed to inject %s: %w", fn, err)
		}

		dstFile := filepath.Join(workspace, fmt.Sprintf("%v.%s.cue", i, fn))
		if err := os.WriteFile(dstFile, data, os.ModePerm); err != nil {
			return fmt.Errorf("failed to write %s: %w", fn, err)
		}
		b.mapSourceToOrigin[dstFile] = file

		files = append(files, dstFile)
	}

	schemaFile := filepath.Join(workspace, fmt.Sprintf("%v.schema.cue", len(b.workspaceFiles[workspace])+1))
	files = append(files, schemaFile)
	if err := os.WriteFile(schemaFile, []byte(apiv1.BundleSchema), os.ModePerm); err != nil {
		return err
	}

	b.workspaceFiles[workspace] = files
	return nil
}

// Build builds a CUE instance for the specified files and returns the CUE value.
// A workspace must be initialised with InitWorkspace before calling this function.
func (b *BundleBuilder) Build(workspace string) (cue.Value, error) {
	var value cue.Value
	cfg := &load.Config{
		Package:   "_",
		DataFiles: true,
	}

	ix := load.Instances(b.workspaceFiles[workspace], cfg)
	if len(ix) == 0 {
		return value, fmt.Errorf("no instances found")
	}

	inst := ix[0]
	if inst.Err != nil {
		return value, fmt.Errorf("instance error: %w", inst.Err)
	}

	v := b.ctx.BuildInstance(inst)
	if v.Err() != nil {
		return value, v.Err()
	}

	if err := v.Validate(cue.Concrete(true)); err != nil {
		return value, err
	}

	return v, nil
}

func (b *BundleBuilder) getInstanceUrl(v cue.Value) string {
	url, _ := v.String()
	if path := strings.TrimPrefix(url, apiv1.LocalPrefix); IsFileUrl(url) && !filepath.IsAbs(path) {
		source := v.Pos().Filename()
		if origin, ok := b.mapSourceToOrigin[source]; ok {
			source = origin
		}
		url = apiv1.LocalPrefix + filepath.Clean(filepath.Join(filepath.Dir(source), path))
	}
	return url
}

// GetBundle returns a Bundle from the bundle CUE value.
func (b *BundleBuilder) GetBundle(v cue.Value) (*Bundle, error) {
	bundleNameValue := v.LookupPath(cue.ParsePath(apiv1.BundleName.String()))
	bundleName, err := bundleNameValue.String()
	if err != nil {
		return nil, fmt.Errorf("lookup %s failed: %w", apiv1.BundleName.String(), bundleNameValue.Err())
	}

	instances := v.LookupPath(cue.ParsePath(apiv1.BundleInstancesSelector.String()))
	if instances.Err() != nil {
		return nil, fmt.Errorf("lookup %s failed: %w", apiv1.BundleInstancesSelector.String(), instances.Err())
	}

	var list []*BundleInstance
	iter, err := instances.Fields(cue.Concrete(true))
	if err != nil {
		return nil, err
	}

	for iter.Next() {
		name := iter.Selector().Unquoted()
		expr := iter.Value()

		vURL := expr.LookupPath(cue.ParsePath(apiv1.BundleModuleURLSelector.String()))
		url := b.getInstanceUrl(vURL)

		vDigest := expr.LookupPath(cue.ParsePath(apiv1.BundleModuleDigestSelector.String()))
		digest, _ := vDigest.String()

		vVersion := expr.LookupPath(cue.ParsePath(apiv1.BundleModuleVersionSelector.String()))
		version, _ := vVersion.String()

		vNamespace := expr.LookupPath(cue.ParsePath(apiv1.BundleNamespaceSelector.String()))
		namespace, _ := vNamespace.String()

		values := expr.LookupPath(cue.ParsePath(apiv1.BundleValuesSelector.String()))

		list = append(list, &BundleInstance{
			Bundle:    bundleName,
			Name:      name,
			Namespace: namespace,
			Module: apiv1.ModuleReference{
				Repository: url,
				Version:    version,
				Digest:     digest,
			},
			Values: values,
		})
	}

	return &Bundle{
		Name:      bundleName,
		Instances: list,
	}, nil
}
