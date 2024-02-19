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

package fetcher

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
)

type Local struct {
	src           string
	dst           string
	requiredFiles []string
}

// NewLocal creates a local Fetcher for the given module.
func NewLocal(src, dst string) *Local {
	requiredFiles := []string{
		path.Join(src, "cue.mod", "module.cue"),
		path.Join(src, "timoni.cue"),
		path.Join(src, "values.cue"),
	}
	return &Local{
		src:           src,
		dst:           dst,
		requiredFiles: requiredFiles,
	}
}

func (f *Local) GetModuleRoot() string {
	return filepath.Join(f.dst, "module")
}

func (f *Local) Fetch() (*apiv1.ModuleReference, error) {
	if fs, err := os.Stat(f.src); err != nil || !fs.IsDir() {
		return nil, fmt.Errorf("module not found at path %s", f.src)
	}

	modFile := path.Join(f.src, "cue.mod", "module.cue")
	timoniFile := path.Join(f.src, "timoni.cue")
	valuesFile := path.Join(f.src, "values.cue")

	for _, requiredFile := range []string{modFile, timoniFile, valuesFile} {
		if _, err := os.Stat(requiredFile); err != nil {
			return nil, fmt.Errorf("required file not found: %s", requiredFile)
		}
	}

	mr := apiv1.ModuleReference{
		Repository: f.src,
		Version:    engine.DefaultDevelVersion,
		Digest:     "unknown",
	}

	return &mr, engine.CopyModule(f.src, f.GetModuleRoot())
}
