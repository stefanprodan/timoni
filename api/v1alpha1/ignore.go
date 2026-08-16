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

package v1alpha1

const (
	IgnoreFile = "timoni.ignore"

	// TestFilePattern matches the CUE files holding a module's test cases.
	// These files are an input to 'timoni mod test' and are always excluded
	// from the published module, regardless of the module's ignore rules.
	TestFilePattern = "*_test.cue"

	DefaultIgnorePatterns = `# VCS
.git/
.gitignore
.gitmodules
.gitattributes

# Go
vendor/
go.mod
go.sum

# CUE
*_tool.cue
*_test.cue
debug_values.cue
`
)
