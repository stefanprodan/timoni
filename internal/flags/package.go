package flags

type Package string

func (f *Package) String() string {
	if f == nil || string(*f) == "" {
		return f.Default()
	}
	return string(*f)
}

func (f *Package) Set(str string) error {
	*f = Package(str)
	return nil
}

func (f *Package) Type() string {
	return "package"
}

func (f *Package) Default() string {
	return "main"
}

func (f *Package) Shorthand() string {
	return "p"
}

func (f *Package) Description() string {
	return "The name of the module's package used for building the templates."
}
