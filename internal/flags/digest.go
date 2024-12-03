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
