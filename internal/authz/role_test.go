package authz

import "testing"

func TestRoleValidation(t *testing.T) {
	for _, role := range []string{PlatformRoleAdmin, PlatformRoleUser} {
		if !IsPlatformRole(role) {
			t.Fatalf("expected platform role %q to be valid", role)
		}
	}
	for _, role := range []string{ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer} {
		if !IsProjectRole(role) {
			t.Fatalf("expected project role %q to be valid", role)
		}
	}
	if IsPlatformRole(ProjectRoleOwner) {
		t.Fatal("project owner must not be accepted as a platform role")
	}
	if IsProjectRole(PlatformRoleAdmin) {
		t.Fatal("platform admin must not be accepted as a project role")
	}
}

func TestNormalizeProjectRole(t *testing.T) {
	if got := NormalizeProjectRole(ProjectRoleAdmin); got != ProjectRoleAdmin {
		t.Fatalf("expected admin, got %q", got)
	}
	if got := NormalizeProjectRole("unknown"); got != ProjectRoleViewer {
		t.Fatalf("expected viewer fallback, got %q", got)
	}
}
