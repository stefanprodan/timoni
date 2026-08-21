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

package flags

type Package string

func (f *Package) String() string {
	if f == nil || string(*f) == "" {
		return f.Default()
	}
	return string(*f)
}

func (f *Package) Set(str string) error {
	*f = Package(str)
	return nil
}

func (f *Package) Type() string {
	return "package"
}

func (f *Package) Default() string {
	return "main"
}

func (f *Package) Shorthand() string {
	return "p"
}

func (f *Package) Description() string {
	return "The name of the module's package used for building the templates."
}
