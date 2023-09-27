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

package oci

import (
	"context"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

// Options returns the crane options for the given context.
func Options(ctx context.Context, credentials string) []crane.Option {
	var opts []crane.Option
	opts = append(opts, crane.WithUserAgent(apiv1.UserAgent), crane.WithContext(ctx))

	if credentials != "" {
		var authConfig authn.AuthConfig
		parts := strings.SplitN(credentials, ":", 2)

		if len(parts) == 1 {
			authConfig = authn.AuthConfig{RegistryToken: parts[0]}
		} else {
			authConfig = authn.AuthConfig{Username: parts[0], Password: parts[1]}
		}

		opts = append(opts, crane.WithAuth(authn.FromConfig(authConfig)))
	}

	return opts
}
