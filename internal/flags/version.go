package flags

import (
	"github.com/Masterminds/semver/v3"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

type Version string

func (f *Version) String() string {
	return string(*f)
}

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
