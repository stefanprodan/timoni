package testutils

// Implements checks if the object implements the interface
func Implements[T any](obj any) bool {
	_, ok := obj.(T)
	return ok
}
