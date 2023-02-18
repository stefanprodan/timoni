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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/encoding/yaml"
	"github.com/fluxcd/pkg/runtime/transform"
	"github.com/fluxcd/pkg/ssa"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	defaultPackage    = "main"
	defaultValuesName = "values"
	defaultValuesFile = "values.cue"
	defaultOutputExp  = "timoni.objects"
)

// Builder compiles CUE definitions to Kubernetes objects.
type Builder struct {
	ctx        *cue.Context
	moduleRoot string
	pkgName    string
	pkgPath    string
	name       string
	namespace  string
}

// NewBuilder creates a Builder for the given module and package.
func NewBuilder(ctx *cue.Context, name, namespace, moduleRoot, pkgName string) *Builder {
	if ctx == nil {
		ctx = cuecontext.New()
	}
	b := &Builder{
		ctx:        ctx,
		moduleRoot: moduleRoot,
		pkgName:    pkgName,
		pkgPath:    moduleRoot,
		name:       name,
		namespace:  namespace,
	}
	if pkgName != defaultPackage {
		b.pkgPath = filepath.Join(moduleRoot, pkgName)
	}
	return b
}

// MergeValuesFile merges the given values overlays into values.cue.
func (b *Builder) MergeValuesFile(overlays []string) error {
	vFinalMap := make(map[string]interface{})
	defaultFile := filepath.Join(b.pkgPath, defaultValuesFile)

	vDefaultMap, err := b.valuesFromFile(defaultFile)
	if err != nil {
		return fmt.Errorf("invalid values in %s, error: %w", defaultFile, err)
	}
	vFinalMap = transform.MergeMaps(vFinalMap, vDefaultMap)

	for _, overlay := range overlays {
		vOverlayMap, err := b.valuesFromFile(overlay)
		if err != nil {
			return fmt.Errorf("invalid values in %s, error: %w", overlay, err)
		}

		vFinalMap = transform.MergeMaps(vFinalMap, vOverlayMap)
	}

	vFinalData, err := json.Marshal(vFinalMap)
	if err != nil {
		return fmt.Errorf("mergeing values failed, error: %w", err)
	}

	vFinal := b.ctx.CompileString(string(vFinalData))
	if vFinal.Err() != nil {
		return fmt.Errorf("compiling final alues failed, error: %w", vFinal.Err())
	}

	cueGen := fmt.Sprintf("package %s\n%s: %v", b.pkgName, defaultValuesName, vFinal)

	//logger.Println(string(vFinalData))
	//logger.Println(cueGen)

	// overwrite the values.cue file with the merged values (concrete)
	if err := os.MkdirAll(b.moduleRoot, os.ModePerm); err != nil {
		return err
	}
	return os.WriteFile(defaultFile, []byte(cueGen), 0644)
}

// Build builds a CUE instances for the specified package and returns the CUE value.
func (b *Builder) Build() (cue.Value, error) {
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
		TagVars: map[string]load.TagVar{},
	}

	ix := load.Instances([]string{}, cfg)
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

// GetObjects coverts the CUE value to Kubernetes unstructured objects.
func (b *Builder) GetObjects(value cue.Value) ([]*unstructured.Unstructured, error) {
	expr := value.LookupPath(cue.ParsePath(defaultOutputExp))
	if expr.Err() != nil {
		return nil, fmt.Errorf("lookup %s failed, error: %w", defaultOutputExp, expr.Err())
	}

	switch expr.Kind() {
	case cue.ListKind:
		items, err := expr.List()
		if err != nil {
			return nil, fmt.Errorf("listing objects failed, error: %w", err)
		}

		data, err := yaml.EncodeStream(items)
		if err != nil {
			return nil, fmt.Errorf("encoding objects to YAML failed, error: %w", err)
		}
		return ssa.ReadObjects(bytes.NewReader(data))
	default:
		return nil, fmt.Errorf("objects are not of type cue.ListKind, got %v", value.Kind())
	}
}

// GetDefaultValues extracts the default values from the module.
func (b *Builder) GetDefaultValues() (string, error) {
	filePath := filepath.Join(b.pkgPath, defaultValuesFile)
	var value cue.Value
	vData, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	value = b.ctx.CompileBytes(vData)
	if value.Err() != nil {
		return "", value.Err()
	}

	expr := value.LookupPath(cue.ParsePath(defaultValuesName))
	if expr.Err() != nil {
		return "", fmt.Errorf("lookup %s failed, error: %w", defaultValuesName, expr.Err())
	}

	return fmt.Sprintf("%v", expr.Eval()), nil
}

// GetModuleName returns the module name as defined in 'cue.mod/module.cue'.
func (b *Builder) GetModuleName() (string, error) {
	filePath := filepath.Join(b.moduleRoot, "cue.mod", "module.cue")
	var value cue.Value
	vData, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	value = b.ctx.CompileBytes(vData)
	if value.Err() != nil {
		return "", value.Err()
	}

	expr := value.LookupPath(cue.ParsePath("module"))
	if expr.Err() != nil {
		return "", fmt.Errorf("lookup module name failed, error: %w", expr.Err())
	}

	mod, err := expr.String()
	if expr.Err() != nil {
		return "", fmt.Errorf("lookup module name failed, error: %w", expr.Err())
	}

	return mod, nil
}

// GetValues extracts the values from the build result.
func (b *Builder) GetValues(value cue.Value) (string, error) {
	expr := value.LookupPath(cue.ParsePath(defaultValuesName))
	if expr.Err() != nil {
		return "", fmt.Errorf("lookup %s failed, error: %w", defaultValuesName, expr.Err())
	}

	return fmt.Sprintf("%v", expr.Eval()), nil
}

func (b *Builder) valuesFromFile(filePath string) (map[string]interface{}, error) {
	vData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	vObj := b.ctx.CompileBytes(vData)
	if vObj.Err() != nil {
		return nil, vObj.Err()
	}

	vValue := vObj.LookupPath(cue.ParsePath(defaultValuesName))
	if vValue.Err() != nil {
		return nil, vObj.Err()
	}

	vJSON, err := vValue.MarshalJSON()
	if err != nil {
		return nil, err
	}

	vMap := make(map[string]interface{})
	err = json.Unmarshal(vJSON, &vMap)
	if err != nil {
		return nil, err
	}

	return vMap, nil
}
