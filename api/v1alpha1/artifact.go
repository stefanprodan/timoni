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
	// ArtifactPrefix is the prefix used for OpenContainers artifact address.
	ArtifactPrefix = "oci://"

	// UserAgent is the agent name used for OpenContainers artifact operations.
	UserAgent = "timoni/v1"

	// ConfigMediaType is the OpenContainers artifact media type for the config layer.
	ConfigMediaType = "application/vnd.timoni.config.v1+json"

	// ContentMediaType is the OpenContainers artifact media type for the content layer.
	ContentMediaType = "application/vnd.timoni.content.v1.tar+gzip"

	// ContentTypeAnnotation is the annotation key used on OpenContainers artifact
	// layers for specified the type of content included in the tarball.
	ContentTypeAnnotation = "sh.timoni.content.type"

	// AnyContentType is the default value of ContentTypeAnnotation.
	AnyContentType = ""

	// TimoniModContentType is the value of ContentTypeAnnotation for setting the
	// layer type to Timoni module content.
	TimoniModContentType = "module"

	// TimoniModVendorContentType is the value of ContentTypeAnnotation for setting the
	// layer type to Timoni module vendored CUE schemas.
	TimoniModVendorContentType = "module/vendor"

	// CueModGenContentType is the value of ContentTypeAnnotation for setting the
	// content to CUE generated schemas.
	CueModGenContentType = "cue.mod/gen"

	// CueModPkgContentType is the value of ContentTypeAnnotation for setting the
	// content to CUE schemas.
	CueModPkgContentType = "cue.mod/pkg"

	// SourceAnnotation is the OpenContainers annotation for specifying
	// the upstream source URL of an artifact.
	SourceAnnotation = "org.opencontainers.image.source"

	// RevisionAnnotation is the OpenContainers annotation for specifying
	// the upstream source revision of an artifact.
	RevisionAnnotation = "org.opencontainers.image.revision"

	// VersionAnnotation is the OpenContainers annotation for specifying
	// the semantic version of an artifact.
	VersionAnnotation = "org.opencontainers.image.version"

	// CreatedAnnotation is the OpenContainers annotation for specifying
	// the build date and time on an artifact (RFC 3339).
	CreatedAnnotation = "org.opencontainers.image.created"
)

// ArtifactReference contains the information necessary to locate
// an artifact in the container registry.
type ArtifactReference struct {
	// Repository is the OpenContainers artifact repo name in the format
	// 'oci://<reg.host>/<org>/<repo>'.
	Repository string `json:"repository"`

	// Tag is the OpenContainers artifact tag name.
	Tag string `json:"tag"`

	// Digest of the OpenContainers artifact in the format '<sha-type>:<hex>'.
	Digest string `json:"digest"`
}
