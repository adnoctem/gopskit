package helpers

// Ptr returns a pointer to the given value. It's most useful for taking the address of a
// literal or the result of a function call, neither of which are directly addressable in Go.
func Ptr[T any](v T) *T {
	return &v
}
