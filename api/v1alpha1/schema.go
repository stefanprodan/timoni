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
	"fmt"
	"regexp"

	"github.com/stefanprodan/timoni/schemas"
)

// pkgClauseRegex matches the 'package v1alpha1' clause line.
var pkgClauseRegex = regexp.MustCompile(`(?m)^package ` + schemaPackage + `\b.*\n`)

// The Timoni CUE schemas are defined once under
// schemas/timoni.sh/core/v1alpha1 where they are published as the
// importable 'timoni.sh/core/v1alpha1' package. The schema strings
// below are derived from those embedded files at startup, so that
// the Go API and the CUE package never drift apart.

var (
	// BundleSchema defines the v1alpha1 CUE schema for Timoni's bundle API.
	BundleSchema = mustInlineSchema("bundle.cue", "bundle: #Bundle")

	// RuntimeSchema defines the v1alpha1 CUE schema for Timoni's runtime API.
	RuntimeSchema = mustInlineSchema("runtime.cue", "")

	// InstanceSchema defines the v1alpha1 CUE schema for Timoni's instance API.
	InstanceSchema = mustInlineSchema("timoni.cue", "timoni: #Timoni")
)

// mustInlineSchema reads an embedded core schema file and returns its
// content as an anonymous-package CUE snippet: the 'package v1alpha1'
// clause is stripped so the definitions can be loaded into Timoni's
// internal '_' package, and the given root binding is appended when set.
func mustInlineSchema(file, binding string) string {
	data, err := schemas.FS.ReadFile(schemaDir + "/" + file)
	if err != nil {
		panic(fmt.Sprintf("failed to load schema %s: %v", file, err))
	}

	content := string(data)
	loc := pkgClauseRegex.FindStringIndex(content)
	if loc == nil {
		panic(fmt.Sprintf("schema %s: missing 'package %s' clause", file, schemaPackage))
	}

	schema := content[loc[1]:]
	if binding != "" {
		schema += "\n" + binding + "\n"
	}
	return schema
}

const (
	// schemaPackage is the CUE package name of the published schemas.
	schemaPackage = "v1alpha1"

	// schemaDir is the embedded path of the published schemas.
	schemaDir = "timoni.sh/core/v1alpha1"
)
