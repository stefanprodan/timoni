package flags

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

type Version string

func (f *Version) String() string {
	return string(*f)
}

// ValidateModuleVersion validates a Timoni module version and OCI tag.
func ValidateModuleVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version is required")
	}
	if strings.Contains(version, "+") {
		return fmt.Errorf("version build metadata is not supported")
	}
	if _, err := semver.StrictNewVersion(version); err != nil {
		return fmt.Errorf("version is not in semver format: %w", err)
	}
	return nil
}

// Set validates a generic semantic version flag.
func (f *Version) Set(str string) error {
	if str != "" && str != apiv1.LatestVersion {
		if _, err := semver.StrictNewVersion(str); err != nil {
			return err
		}
	}
	*f = Version(str)
	return nil
}

func (f *Version) Type() string {
	return "version"
}

func (f *Version) Shorthand() string {
	return "v"
}

func (f *Version) Description() string {
	return "The version of the module e.g. '1.0.0' or '1.0.0-rc.1'."
}
