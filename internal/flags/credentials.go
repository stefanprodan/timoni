package flags

import (
	"fmt"
)

type Credentials string

func (f *Credentials) String() string {
	return string(*f)
}

func (f *Credentials) Set(str string) error {
	*f = Credentials(str)
	return nil
}

func (f *Credentials) Type() string {
	return "creds"
}

func (f *Credentials) Description() string {
	return fmt.Sprintf("The credentials for the container registry in the format '<username>[:<password>]'.")
}
