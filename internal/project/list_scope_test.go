package project

import (
	"errors"
	"testing"
)

func TestResolveListVisibilityDefaultsToRelatedForEveryRole(t *testing.T) {
	for _, platformAdmin := range []bool{false, true} {
		visibility, err := ResolveListVisibility("", platformAdmin)
		if err != nil || visibility != ListVisibilityRelated {
			t.Fatalf("platformAdmin=%t visibility=%q error=%v", platformAdmin, visibility, err)
		}
	}
}

func TestResolveListVisibilityRequiresExplicitAdministratorAccessForAll(t *testing.T) {
	if _, err := ResolveListVisibility("all", false); !errors.Is(err, ErrListVisibilityForbidden) {
		t.Fatalf("non-admin all visibility error = %v", err)
	}
	visibility, err := ResolveListVisibility(" ALL ", true)
	if err != nil || visibility != ListVisibilityAll {
		t.Fatalf("admin all visibility=%q error=%v", visibility, err)
	}
	if _, err := ResolveListVisibility("mine", true); !errors.Is(err, ErrListVisibilityInvalid) {
		t.Fatalf("invalid visibility error = %v", err)
	}
}
