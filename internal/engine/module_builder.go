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

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

const (
	defaultPackage    = "main"
	defaultValuesFile = "values.cue"
)

// ModuleBuilder compiles CUE definitions to Kubernetes objects.
type ModuleBuilder struct {
	ctx        *cue.Context
	moduleRoot string
	pkgName    string
	pkgPath    string
	name       string
	namespace  string
}

// NewModuleBuilder creates a ModuleBuilder for the given module and package.
func NewModuleBuilder(ctx *cue.Context, name, namespace, moduleRoot, pkgName string) *ModuleBuilder {
	if ctx == nil {
		ctx = cuecontext.New()
	}
	b := &ModuleBuilder{
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

// MergeValuesFile merges the given values overlays into the module's root values.cue.
func (b *ModuleBuilder) MergeValuesFile(overlays []string) error {
	vb := NewValuesBuilder(b.ctx)
	defaultFile := filepath.Join(b.pkgPath, defaultValuesFile)

	finalVal, err := vb.MergeValues(overlays, defaultFile)
	if err != nil {
		return err
	}

	cueGen := fmt.Sprintf("package %s\n%s: %v", b.pkgName, apiv1.ValuesSelector, finalVal)

	// overwrite the values.cue file with the merged values
	if err := os.MkdirAll(b.moduleRoot, os.ModePerm); err != nil {
		return err
	}
	return os.WriteFile(defaultFile, []byte(cueGen), 0644)
}

// Build builds a CUE instances for the specified package and returns the CUE value.
func (b *ModuleBuilder) Build() (cue.Value, error) {
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

// GetAPIVersion returns the list of API version of the Timoni's CUE definition.
func (b *ModuleBuilder) GetAPIVersion(value cue.Value) (string, error) {
	ver := value.LookupPath(cue.ParsePath(apiv1.APIVersionSelector.String()))
	if ver.Err() != nil {
		return "", fmt.Errorf("lookup %s failed, error: %w", apiv1.APIVersionSelector, ver.Err())
	}
	return ver.String()
}

// GetApplySets returns the list of Kubernetes unstructured objects to be applied in steps.
func (b *ModuleBuilder) GetApplySets(value cue.Value) ([]ResourceSet, error) {
	steps := value.LookupPath(cue.ParsePath(apiv1.ApplySelector.String()))
	if steps.Err() != nil {
		return nil, fmt.Errorf("lookup %s failed, error: %w", apiv1.ApplySelector, steps.Err())
	}
	return GetResources(steps)
}

// GetDefaultValues extracts the default values from the module.
func (b *ModuleBuilder) GetDefaultValues() (string, error) {
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

	expr := value.LookupPath(cue.ParsePath(apiv1.ValuesSelector.String()))
	if expr.Err() != nil {
		return "", fmt.Errorf("lookup %s failed, error: %w", apiv1.ValuesSelector, expr.Err())
	}

	return fmt.Sprintf("%v", expr.Eval()), nil
}

// GetModuleName returns the module name as defined in 'cue.mod/module.cue'.
func (b *ModuleBuilder) GetModuleName() (string, error) {
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
func (b *ModuleBuilder) GetValues(value cue.Value) (string, error) {
	expr := value.LookupPath(cue.ParsePath(apiv1.ValuesSelector.String()))
	if expr.Err() != nil {
		return "", fmt.Errorf("lookup %s failed, error: %w", apiv1.ValuesSelector, expr.Err())
	}

	return fmt.Sprintf("%v", expr.Eval()), nil
}
