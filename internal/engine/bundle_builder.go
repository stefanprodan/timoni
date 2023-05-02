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

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// BundleBuilder compiles CUE definitions to Go Bundle objects.
type BundleBuilder struct {
	ctx     *cue.Context
	pkgPath string
	files   []string
}

type Bundle struct {
	Name      string
	Instances []BundleInstance
}

type BundleInstance struct {
	Bundle    string
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
		ctx:   ctx,
		files: files,
	}
	return b
}

// Build builds a CUE instance for the specified files and returns the CUE value.
func (b *BundleBuilder) Build() (cue.Value, error) {
	var value cue.Value
	cfg := &load.Config{
		Package:   "_",
		DataFiles: true,
	}

	ix := load.Instances(b.files, cfg)
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

// GetBundle returns a Bundle from the bundle CUE value.
func (b *BundleBuilder) GetBundle(v cue.Value) (*Bundle, error) {
	bundleNameValue := v.LookupPath(cue.ParsePath(apiv1.BundleName.String()))
	bundleName, err := bundleNameValue.String()
	if err != nil {
		return nil, fmt.Errorf("lookup %s failed, error: %w", apiv1.BundleName.String(), bundleNameValue.Err())
	}

	instances := v.LookupPath(cue.ParsePath(apiv1.BundleInstancesSelector.String()))
	if instances.Err() != nil {
		return nil, fmt.Errorf("lookup %s failed, error: %w", apiv1.BundleInstancesSelector.String(), instances.Err())
	}

	var list []BundleInstance
	iter, err := instances.Fields(cue.Concrete(true))
	if err != nil {
		return nil, err
	}

	for iter.Next() {
		name := iter.Selector().String()
		expr := iter.Value()

		vURL := expr.LookupPath(cue.ParsePath(apiv1.BundleModuleURLSelector.String()))
		url, _ := vURL.String()

		vDigest := expr.LookupPath(cue.ParsePath(apiv1.BundleModuleDigestSelector.String()))
		digest, _ := vDigest.String()

		vVersion := expr.LookupPath(cue.ParsePath(apiv1.BundleModuleVersionSelector.String()))
		version, _ := vVersion.String()

		vNamespace := expr.LookupPath(cue.ParsePath(apiv1.BundleNamespaceSelector.String()))
		namespace, _ := vNamespace.String()

		values := expr.LookupPath(cue.ParsePath(apiv1.BundleValuesSelector.String()))

		list = append(list, BundleInstance{
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
