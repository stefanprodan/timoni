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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

const (
	defaultPackage      = "main"
	defaultValuesFile   = "values.cue"
	defaultSchemaFile   = "timoni.schema.cue"
	DefaultDevelVersion = "0.0.0-devel"

	// The default Kubernetes version must be kept in sync with go.mod.
	defaultKubeVersion = "1.27.5"
)

// ModuleBuilder compiles CUE definitions to Kubernetes objects.
type ModuleBuilder struct {
	ctx           *cue.Context
	moduleRoot    string
	pkgName       string
	pkgPath       string
	name          string
	namespace     string
	moduleVersion string
	kubeVersion   string
}

// NewModuleBuilder creates a ModuleBuilder for the given module and package.
func NewModuleBuilder(ctx *cue.Context, name, namespace, moduleRoot, pkgName string) *ModuleBuilder {
	if ctx == nil {
		ctx = cuecontext.New()
	}
	b := &ModuleBuilder{
		ctx:           ctx,
		moduleRoot:    moduleRoot,
		pkgName:       pkgName,
		pkgPath:       moduleRoot,
		name:          name,
		namespace:     namespace,
		moduleVersion: DefaultDevelVersion,
		kubeVersion:   defaultKubeVersion,
	}

	if kv := os.Getenv("TIMONI_KUBE_VERSION"); kv != "" {
		b.kubeVersion = kv
	}

	if pkgName != defaultPackage {
		b.pkgPath = filepath.Join(moduleRoot, pkgName)
	}
	return b
}

// MergeValuesFile merges the given values overlays into the module's root values.cue.
func (b *ModuleBuilder) MergeValuesFile(overlays [][]byte) error {
	vb := NewValuesBuilder(b.ctx)
	defaultFile := filepath.Join(b.pkgPath, defaultValuesFile)

	finalVal, err := vb.MergeValues(overlays, defaultFile)
	if err != nil {
		return err
	}

	if err := finalVal.Err(); err != nil {
		return err
	}

	cueGen := fmt.Sprintf("package %s\n%s: %v", b.pkgName, apiv1.ValuesSelector, finalVal)

	// overwrite the values.cue file with the merged values
	if err := os.MkdirAll(b.moduleRoot, os.ModePerm); err != nil {
		return err
	}
	return os.WriteFile(defaultFile, []byte(cueGen), 0644)
}

// WriteValuesFileWithDefaults merges the module's root values.cue with the supplied value.
func (b *ModuleBuilder) WriteValuesFileWithDefaults(val cue.Value) error {
	valData := []byte(fmt.Sprintf("%s: %v", apiv1.ValuesSelector.String(), val))

	vb := NewValuesBuilder(b.ctx)
	defaultFile := filepath.Join(b.pkgPath, defaultValuesFile)

	finalVal, err := vb.MergeValues([][]byte{valData}, defaultFile)
	if err != nil {
		return err
	}

	if err := finalVal.Err(); err != nil {
		return err
	}

	cueGen := fmt.Sprintf("package %s\n%s: %v", b.pkgName, apiv1.ValuesSelector, finalVal)

	// overwrite the values.cue file with the merged values
	if err := os.MkdirAll(b.moduleRoot, os.ModePerm); err != nil {
		return err
	}
	return os.WriteFile(defaultFile, []byte(cueGen), 0644)
}

// WriteSchemaFile generates the module's instance schema.
func (b *ModuleBuilder) WriteSchemaFile() error {
	if fs, err := os.Stat(b.pkgPath); err != nil || !fs.IsDir() {
		return fmt.Errorf("cannot find package %s", b.pkgPath)
	}

	cueGen := fmt.Sprintf("package %s\n%v", b.pkgName, apiv1.InstanceSchema)

	return os.WriteFile(filepath.Join(b.pkgPath, defaultSchemaFile), []byte(cueGen), 0644)
}

// SetVersionInfo allows setting the Timoni module version and Kubernetes version,
// which are injected at build time as optional CUE tags.
func (b *ModuleBuilder) SetVersionInfo(moduleVersion, kubeVersion string) {
	if moduleVersion != "" {
		b.moduleVersion = moduleVersion
	}

	if kubeVersion != "" {
		b.kubeVersion = kubeVersion
	}
}

// Build builds the Timoni instance for the specified module and returns its CUE value.
// If the instance validation fails, the returned error may represent more than one error,
// retrievable with errors.Errors.
func (b *ModuleBuilder) Build(tags ...string) (cue.Value, error) {
	var value cue.Value
	cfg := &load.Config{
		AcceptLegacyModules: true,
		ModuleRoot:          b.moduleRoot,
		Package:             b.pkgName,
		Dir:                 b.pkgPath,
		DataFiles:           true,
		Tags: []string{
			"name=" + b.name,
			"namespace=" + b.namespace,
		},
		TagVars: map[string]load.TagVar{
			"moduleVersion": {
				Func: func() (ast.Expr, error) {
					return ast.NewString(b.moduleVersion), nil
				},
			},
			"kubeVersion": {
				Func: func() (ast.Expr, error) {
					return ast.NewString(b.kubeVersion), nil
				},
			},
		},
	}

	if len(tags) > 0 {
		cfg.Tags = append(cfg.Tags, tags...)
	}

	modInstances := load.Instances([]string{}, cfg)
	if len(modInstances) == 0 {
		return value, errors.New("no instances found")
	}

	modInstance := modInstances[0]
	if modInstance.Err != nil {
		return value, fmt.Errorf("instance error: %w", modInstance.Err)
	}

	modValue := b.ctx.BuildInstance(modInstance)
	if modValue.Err() != nil {
		return value, modValue.Err()
	}

	// Extract the Timoni instance from the build value.
	instance := modValue.LookupPath(cue.ParsePath(apiv1.InstanceSelector.String()))
	if instance.Err() != nil {
		return modValue, fmt.Errorf("lookup %s failed: %w", apiv1.InstanceSelector, instance.Err())
	}

	// Validate the Timoni instance which should be concrete and final.
	if err := instance.Validate(cue.Concrete(true), cue.Final()); err != nil {
		return modValue, err
	}

	return modValue, nil
}

// GetAPIVersion returns the list of API version of the Timoni's CUE definition.
func (b *ModuleBuilder) GetAPIVersion(value cue.Value) (string, error) {
	ver := value.LookupPath(cue.ParsePath(apiv1.APIVersionSelector.String()))
	if ver.Err() != nil {
		return "", fmt.Errorf("lookup %s failed: %w", apiv1.APIVersionSelector, ver.Err())
	}
	return ver.String()
}

// GetApplySets returns the list of Kubernetes unstructured objects to be applied in steps.
func (b *ModuleBuilder) GetApplySets(value cue.Value) ([]ResourceSet, error) {
	steps := value.LookupPath(cue.ParsePath(apiv1.ApplySelector.String()))
	if steps.Err() != nil {
		return nil, fmt.Errorf("lookup %s failed: %w", apiv1.ApplySelector, steps.Err())
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
		return "", fmt.Errorf("lookup %s failed: %w", apiv1.ValuesSelector, expr.Err())
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
		return "", fmt.Errorf("lookup module name failed: %w", expr.Err())
	}

	mod, err := expr.String()
	if err != nil {
		return "", fmt.Errorf("lookup module name failed: %w", err)
	}

	return mod, nil
}

// GetContainerImages extracts the container images referenced in the instance config values.
func (b *ModuleBuilder) GetContainerImages(value cue.Value) ([]string, error) {
	cfgValues := value.LookupPath(cue.ParsePath(apiv1.ConfigValuesSelector.String()))
	if cfgValues.Err() != nil {
		return nil, fmt.Errorf("lookup %s failed: %w", apiv1.ConfigValuesSelector, cfgValues.Err())
	}

	var images []string
	imgExtract := func(v cue.Value) bool {
		switch v.IncompleteKind() {
		case cue.StructKind:
			var img apiv1.ImageReference
			imgVal := reflect.ValueOf(img)
			for i := 0; i < imgVal.Type().NumField(); i++ {
				if tag, ok := imgVal.Type().Field(i).Tag.Lookup("json"); ok {
					if !v.LookupPath(cue.ParsePath(tag)).Exists() {
						return true
					}
				}
			}
			if err := v.Decode(&img); err == nil {
				images = append(images, img.Reference)
			}
		}
		return true
	}

	cfgValues.Walk(imgExtract, nil)

	images = slices.Compact(images)
	slices.Sort(images)

	return images, nil
}
