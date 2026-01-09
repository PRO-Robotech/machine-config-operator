// Package testutil provides common test utilities.
package testutil

import (
	"k8s.io/utils/ptr"
)

// Ptr returns a pointer to the given value.
// Convenience wrapper around k8s.io/utils/ptr.To
func Ptr[T any](v T) *T {
	return ptr.To(v)
}

// StringSlice returns a slice of strings from variadic args.
func StringSlice(s ...string) []string {
	return s
}
