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

import (
	"fmt"
	"strings"
)

type Digest string

func (f *Digest) String() string {
	return string(*f)
}

func (f *Digest) Set(str string) error {
	if str != "" {
		s := strings.Split(str, ":")
		if len(s) != 2 || s[0] == "" || s[1] == "" {
			return fmt.Errorf("digest must be in the format <sha-type>:<hex>")
		}
	}
	*f = Digest(str)
	return nil
}

func (f *Digest) Type() string {
	return "digest"
}

func (f *Digest) Shorthand() string {
	return "d"
}

func (f *Digest) Description() string {
	return "The digest of the module e.g. sha256:3f29e1b2b05f8371595dc761fed8e8b37544b38d56dfce81a551b46c82f2f56b."
}
