package policies_test

import "testing"

func must(err error, t *testing.T) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func ptr[T any](value T) *T {
	return &value
}
