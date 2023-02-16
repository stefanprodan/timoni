package v1alpha1

// ModuleReference contains the information necessary to locate
// a module's OCI artifact in the registry.
type ModuleReference struct {
	// Name of the module.
	Name string `json:"name"`

	// Repository is the OCI artifact repo name in the format
	// 'oci://<reg.host>/<org>/<repo>'.
	Repository string `json:"repository"`

	// Version is the OCI artifact tag in strict semver format.
	Version string `json:"version"`

	// Digest of the OCI artifact in the format '<sha-type>:<hex>'.
	Digest string `json:"digest"`
}
