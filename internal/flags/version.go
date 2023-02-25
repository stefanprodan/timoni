package flags

import (
	"fmt"

	"github.com/Masterminds/semver/v3"

	"github.com/stefanprodan/timoni/internal/engine"
)

type Version string

func (f *Version) String() string {
	return string(*f)
}

func (f *Version) Set(str string) error {
	if str != "" && str != engine.LatestTag {
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
	return fmt.Sprintf("The version of the module e.g. '1.0.0' or '1.0.0-rc.1'.")
}
