/*
Copyright 2024 Stefan Prodan

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

package runtimebuild

import (
	"os"

	"cuelang.org/go/cue/cuecontext"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/errors"
)

type Options struct {
	KubeConfigFlags *genericclioptions.ConfigFlags
}

func BuildFiles(opts Options, paths ...string) (*apiv1.Runtime, error) {
	defaultRuntime := apiv1.DefaultRuntime(*opts.KubeConfigFlags.Context)
	if len(paths) == 0 {
		return defaultRuntime, nil
	}

	tmpDir, err := os.MkdirTemp("", apiv1.FieldManager)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	rb := engine.NewRuntimeBuilder(cuecontext.New(), paths)

	if err := rb.InitWorkspace(tmpDir); err != nil {
		return nil, errors.Describe(tmpDir, "failed to init runtime", err)
	}

	v, err := rb.Build()
	if err != nil {
		return nil, errors.Describe(tmpDir, "failed to parse runtime", err)
	}

	rt, err := rb.GetRuntime(v)
	if err != nil {
		return nil, err
	}

	if len(rt.Clusters) == 0 {
		rt.Clusters = defaultRuntime.Clusters
	}
	return rt, nil
}
