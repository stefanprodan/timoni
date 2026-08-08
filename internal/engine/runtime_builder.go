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
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/encoding/json"
	"cuelang.org/go/encoding/yaml"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// RuntimeBuilder compiles CUE definitions to Go Runtime objects.
type RuntimeBuilder struct {
	ctx               *cue.Context
	files             []string
	root              string
	workdir           string
	workspacesFiles   map[string][]string
	workspacesSources map[string]map[string][]byte
}

// NewRuntimeBuilder creates a RuntimeBuilder for the given module and package.
func NewRuntimeBuilder(ctx *cue.Context, files []string) *RuntimeBuilder {
	if ctx == nil {
		ctx = cuecontext.New()
	}
	b := &RuntimeBuilder{
		ctx:               ctx,
		files:             files,
		root:              NewVirtualRoot(),
		workspacesFiles:   make(map[string][]string),
		workspacesSources: make(map[string]map[string][]byte),
	}
	return b
}

// WorkspaceDir returns the virtual directory under which the workspace
// files are laid out. The directory does not exist on disk; it serves
// as the base for rendering error positions relative.
func (b *RuntimeBuilder) WorkspaceDir(workspace string) string {
	return filepath.Join(b.root, workspace)
}

// SetWorkdir sets the directory from which the CUE loader discovers the
// cue.mod module root, enabling imports in the runtime definitions. When
// unset, the loader uses the process working directory.
func (b *RuntimeBuilder) SetWorkdir(dir string) {
	b.workdir = dir
}

// InitWorkspace extracts the runtime definitions into the in-memory
// workspace identified by the given name. Nothing is written to disk;
// the workspace files are kept in memory and served to the CUE loader
// as overlays. A workspace must be initialised before calling Build.
func (b *RuntimeBuilder) InitWorkspace(workspace string) error {
	var files []string
	sources := make(map[string][]byte, len(b.files)+1)
	workspaceDir := b.WorkspaceDir(workspace)
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

		data, err := format.Node(node)
		if err != nil {
			return fmt.Errorf("failed to format node %s: %w", fn, err)
		}

		dstFile := filepath.Join(workspaceDir, fmt.Sprintf("%v.%s.cue", i, strings.TrimSuffix(fn, ".cue")))
		sources[dstFile] = data

		files = append(files, dstFile)
	}

	// The runtime files are indexed 0..len(files)-1, so naming the generated
	// schema after len(files) cannot collide with a user file named schema.cue.
	schemaFile := filepath.Join(workspaceDir, fmt.Sprintf("%v.schema.cue", len(b.files)))
	files = append(files, schemaFile)
	sources[schemaFile] = []byte(apiv1.RuntimeSchema)

	b.workspacesFiles[workspace] = files
	b.workspacesSources[workspace] = sources
	return nil
}

// Build builds a CUE instance for the specified workspace and returns the CUE value.
// The workspace files are served to the CUE loader as in-memory overlays.
// A workspace must be initialised with InitWorkspace before calling this function.
func (b *RuntimeBuilder) Build(workspace string) (cue.Value, error) {
	var value cue.Value
	overlay := make(map[string]load.Source, len(b.workspacesSources[workspace]))
	for f, data := range b.workspacesSources[workspace] {
		overlay[f] = load.FromBytes(data)
	}

	cfg := &load.Config{
		Package:   "_",
		DataFiles: true,
		Overlay:   overlay,
		Dir:       b.workdir,
	}

	ix := load.Instances(b.workspacesFiles[workspace], cfg)
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

	return v, nil
}

// GetRuntime returns a Runtime from the specified CUE value.
func (b *RuntimeBuilder) GetRuntime(v cue.Value) (*apiv1.Runtime, error) {
	runtimeNameValue := v.LookupPath(cue.ParsePath(apiv1.RuntimeName.String()))
	runtimeName, err := runtimeNameValue.String()
	if err != nil {
		return nil, fmt.Errorf("lookup %s failed: %w", apiv1.RuntimeName.String(), runtimeNameValue.Err())
	}

	clusters := []apiv1.RuntimeCluster{}
	clustersCue := v.LookupPath(cue.ParsePath(apiv1.RuntimeClustersSelector.String()))
	if clustersCue.Err() == nil {
		iter, err := clustersCue.Fields(cue.Concrete(true))
		if err != nil {
			return nil, err
		}

		for iter.Next() {
			name := iter.Selector().Unquoted()
			expr := iter.Value()

			vGroup := expr.LookupPath(cue.ParsePath("group"))
			group, _ := vGroup.String()

			vkc := expr.LookupPath(cue.ParsePath("kubeContext"))
			kc, _ := vkc.String()

			clusters = append(clusters, apiv1.RuntimeCluster{
				Name:        name,
				Group:       group,
				KubeContext: kc,
			})
		}
	}

	var refs []apiv1.RuntimeResourceRef
	runtimeValuesCue := v.LookupPath(cue.ParsePath(apiv1.RuntimeValuesSelector.String()))
	if runtimeValuesCue.Err() == nil {
		runtimeValues := []apiv1.RuntimeValue{}
		err = runtimeValuesCue.Decode(&runtimeValues)
		if err != nil {
			return nil, fmt.Errorf("values decoding failed: %w", err)
		}

		for _, rv := range runtimeValues {
			ref, err := rv.ToResourceRef()
			if err != nil {
				return nil, fmt.Errorf("value decoding failed: %w", err)
			}

			refs = append(refs, *ref)
		}
	}

	return &apiv1.Runtime{
		Name:     runtimeName,
		Clusters: clusters,
		Refs:     refs,
	}, nil
}
