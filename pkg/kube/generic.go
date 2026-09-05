package kube

// get is a generic helper for the common "single object, error" return shape used by client-go's
// Get calls, removing the repetitive nil-check boilerplate from each per-resource wrapper.
func get[T any](obj *T, err error) (*T, error) {
	if err != nil {
		return nil, err
	}

	return obj, nil
}

// created is the equivalent of get for Create calls, which only need the error propagated.
func created[T any](_ *T, err error) error {
	return err
}

// items is the equivalent of get for List calls: it checks the error, then extracts the Items
// slice from the list object via extract.
func items[L, T any](list *L, err error, extract func(*L) []T) ([]T, error) {
	if err != nil {
		return nil, err
	}

	return extract(list), nil
}
