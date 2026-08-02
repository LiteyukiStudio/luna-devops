package id

import (
	"regexp"
	"testing"
)

func TestNewResourceDatabaseIDsUseRandomOpaqueValues(t *testing.T) {
	t.Parallel()

	for _, prefix := range []string{"prj", "app", "dplt"} {
		prefix := prefix
		t.Run(prefix, func(t *testing.T) {
			t.Parallel()

			first := New(prefix)
			second := New(prefix)
			pattern := regexp.MustCompile("^" + prefix + `_[0-9a-f]{24}$`)
			if !pattern.MatchString(first) || !pattern.MatchString(second) {
				t.Fatalf("generated IDs = %q, %q", first, second)
			}
			if first == second {
				t.Fatalf("generated duplicate ID %q", first)
			}
		})
	}
}
