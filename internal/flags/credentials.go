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

package flags

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
)

const maxCredentialsFileSize = 64 << 10

// Credentials stores container registry credentials supplied by a CLI flag.
type Credentials string

// String returns the credentials in username and optional password format.
func (f *Credentials) String() string {
	return string(*f)
}

// Set stores inline credentials or reads exact credential bytes from an @path.
func (f *Credentials) Set(str string) error {
	if strings.HasPrefix(str, "@@") {
		*f = Credentials(str[1:])
		return nil
	}
	if strings.HasPrefix(str, "@") {
		path := str[1:]
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("reading credentials file %q failed: %w", path, err)
		}
		contents, readErr := io.ReadAll(io.LimitReader(file, maxCredentialsFileSize+1))
		closeErr := file.Close()
		if readErr != nil {
			return fmt.Errorf("reading credentials file %q failed: %w", path, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("reading credentials file %q failed: %w", path, closeErr)
		}
		if len(contents) > maxCredentialsFileSize {
			return fmt.Errorf("credentials file exceeds %d bytes", maxCredentialsFileSize)
		}
		if bytes.HasSuffix(contents, []byte("\r\n")) {
			contents = contents[:len(contents)-2]
		} else {
			contents = bytes.TrimSuffix(contents, []byte("\n"))
		}
		*f = Credentials(contents)
		return nil
	}
	*f = Credentials(str)
	return nil
}

// Type returns the CLI flag name.
func (f *Credentials) Type() string {
	return "creds"
}

// Description returns the CLI flag help text.
func (f *Credentials) Description() string {
	return "The credentials for the container registry as '<username>[:<password>]' or '@<path>' ('@@' escapes a leading '@')."
}
