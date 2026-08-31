package authz

import (
	"reflect"
	"testing"
)

func TestNormalizeKubeScopesAddsReadAndUsesStableOrder(t *testing.T) {
	got, err := NormalizeKubeScopes([]string{KubeScopeConnect, KubeScopeWrite, KubeScopeWrite})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{KubeScopeRead, KubeScopeWrite, KubeScopeConnect}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scopes = %#v, want %#v", got, want)
	}
}

func TestNormalizeKubeScopesRejectsOrdinaryAndWildcardScopes(t *testing.T) {
	for _, scope := range []string{"*", "project:read", "kube:*", ""} {
		if _, err := NormalizeKubeScopes([]string{scope}); err == nil {
			t.Fatalf("scope %q was accepted", scope)
		}
	}
}

func TestRequiredKubeScope(t *testing.T) {
	if got := RequiredKubeScope("watch"); got != KubeScopeRead {
		t.Fatalf("watch scope = %q", got)
	}
	if got := RequiredKubeScope("patch"); got != KubeScopeWrite {
		t.Fatalf("patch scope = %q", got)
	}
	if got := RequiredKubeScope("connect"); got != KubeScopeConnect {
		t.Fatalf("connect scope = %q", got)
	}
}
