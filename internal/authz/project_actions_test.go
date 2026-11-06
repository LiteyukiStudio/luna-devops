package authz

import (
	"slices"
	"testing"
)

func TestProjectActionsAllowedProjectsThePolicyTable(t *testing.T) {
	viewer := ProjectActionsAllowed(PlatformRoleUser, ProjectRoleViewer)
	if !slices.Contains(viewer, ActionProjectRead) || slices.Contains(viewer, ActionProjectWrite) || slices.Contains(viewer, ActionVolumeWrite) {
		t.Fatalf("viewer actions = %v", viewer)
	}
	developer := ProjectActionsAllowed(PlatformRoleUser, ProjectRoleDeveloper)
	if !slices.Contains(developer, ActionProjectWrite) || !slices.Contains(developer, ActionVolumeWrite) || slices.Contains(developer, ActionVolumeExport) {
		t.Fatalf("developer actions = %v", developer)
	}
	if !slices.IsSorted(developer) {
		t.Fatalf("actions are not sorted: %v", developer)
	}
	if got := ProjectActionsAllowed(PlatformRoleUser, ""); len(got) != 0 {
		t.Fatalf("non-member actions = %v", got)
	}
	admin := ProjectActionsAllowed(PlatformRoleAdmin, "")
	if len(admin) != len(projectPolicies) {
		t.Fatalf("platform admin actions = %d, want %d", len(admin), len(projectPolicies))
	}
	for _, action := range admin {
		for _, role := range []string{ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer} {
			if slices.Contains(ProjectActionsAllowed(PlatformRoleUser, role), action) != ProjectRoleAllows(role, action) {
				t.Fatalf("role %s action %s diverges from ProjectRoleAllows", role, action)
			}
		}
	}
}
