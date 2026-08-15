package project

import (
	"errors"
	"testing"
)

func TestResolveListScopeDefaultsToRelatedForEveryRole(t *testing.T) {
	for _, platformAdmin := range []bool{false, true} {
		scope, err := ResolveListScope("", platformAdmin)
		if err != nil || scope != ListScopeRelated {
			t.Fatalf("platformAdmin=%t scope=%q error=%v", platformAdmin, scope, err)
		}
	}
}

func TestResolveListScopeRequiresExplicitAdministratorAccessForAll(t *testing.T) {
	if _, err := ResolveListScope("all", false); !errors.Is(err, ErrListScopeForbidden) {
		t.Fatalf("non-admin all scope error = %v", err)
	}
	scope, err := ResolveListScope(" ALL ", true)
	if err != nil || scope != ListScopeAll {
		t.Fatalf("admin all scope=%q error=%v", scope, err)
	}
	if _, err := ResolveListScope("mine", true); !errors.Is(err, ErrListScopeInvalid) {
		t.Fatalf("invalid scope error = %v", err)
	}
}
