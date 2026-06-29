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

package v1alpha1

import (
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

// TestSchemas verifies that the schema strings derived from the embedded
// timoni.sh/core/v1alpha1 CUE files are loaded as anonymous-package
// snippets (no leftover package clause), expose their root definition,
// carry the expected root binding, and compile.
func TestSchemas(t *testing.T) {
	tests := []struct {
		name    string
		schema  string
		def     string
		binding string
	}{
		{"bundle", BundleSchema, "#Bundle", "bundle: #Bundle"},
		{"runtime", RuntimeSchema, "#Runtime", ""},
		{"instance", InstanceSchema, "#Timoni", "timoni: #Timoni"},
	}

	ctx := cuecontext.New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.schema == "" {
				t.Fatal("schema is empty")
			}
			if strings.Contains(tt.schema, "package "+schemaPackage) {
				t.Errorf("schema still contains the %q clause", "package "+schemaPackage)
			}
			if !strings.Contains(tt.schema, tt.def) {
				t.Errorf("schema does not define %s", tt.def)
			}
			if tt.binding != "" && !strings.Contains(tt.schema, tt.binding) {
				t.Errorf("schema does not bind %q", tt.binding)
			}
			if v := ctx.CompileString(tt.schema); v.Err() != nil {
				t.Errorf("schema does not compile: %v", v.Err())
			}
		})
	}
}
