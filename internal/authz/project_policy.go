package authz

// ProjectPolicy is the single role contract for a project action.
type ProjectPolicy struct {
	AllowedRoles []string
}

var projectPolicies = map[Action]ProjectPolicy{
	ActionProjectRead:      policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer),
	ActionProjectWrite:     policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),
	ActionProjectManage:    policy(ProjectRoleOwner, ProjectRoleAdmin),
	ActionProjectDelete:    policy(ProjectRoleOwner),
	ActionProjectOwnerOnly: policy(ProjectRoleOwner),
	ActionProjectPin:       policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer),

	ActionApplicationRead:   policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer),
	ActionApplicationCreate: policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),
	ActionApplicationUpdate: policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),
	ActionApplicationDelete: policy(ProjectRoleOwner, ProjectRoleAdmin),

	ActionDeploymentRead:     policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer),
	ActionDeploymentUpdate:   policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),
	ActionDeploymentRelease:  policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),
	ActionDeploymentRestart:  policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),
	ActionDeploymentRollback: policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),
	ActionDeploymentDelete:   policy(ProjectRoleOwner, ProjectRoleAdmin),
	ActionDeploymentExec:     policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),

	ActionBuildRead:    policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer),
	ActionBuildTrigger: policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),
	ActionBuildCancel:  policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),
	ActionBuildDelete:  policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),

	ActionGatewayRead:   policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer),
	ActionGatewayManage: policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),
	ActionGatewayDelete: policy(ProjectRoleOwner, ProjectRoleAdmin),

	ActionSecretReadSummary: policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer),
	ActionSecretViewValue:   policy(ProjectRoleOwner, ProjectRoleAdmin),
	ActionSecretUpdate:      policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),

	ActionClusterRead:   policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer),
	ActionClusterUse:    policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),
	ActionClusterManage: policy(ProjectRoleOwner, ProjectRoleAdmin),

	ActionBillingRead:   policy(ProjectRoleOwner, ProjectRoleAdmin),
	ActionBillingAdjust: policy(ProjectRoleOwner),

	ActionGitRead:  policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer),
	ActionGitWrite: policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),

	ActionRegistryRead: policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer),
	ActionRegistryUse:  policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),
	ActionImageWrite:   policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),

	ActionVolumeRead:   policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer),
	ActionVolumeWrite:  policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),
	ActionVolumeImport: policy(ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper),
	ActionVolumeExport: policy(ProjectRoleOwner, ProjectRoleAdmin),
	ActionVolumeDelete: policy(ProjectRoleOwner, ProjectRoleAdmin),
}

func policy(roles ...string) ProjectPolicy {
	return ProjectPolicy{AllowedRoles: roles}
}

// ProjectPolicyForAction returns a defensive copy so callers cannot mutate the
// process-wide authorization contract.
func ProjectPolicyForAction(action Action) (ProjectPolicy, bool) {
	value, ok := projectPolicies[action]
	if !ok {
		return ProjectPolicy{}, false
	}
	value.AllowedRoles = append([]string(nil), value.AllowedRoles...)
	return value, true
}

func ProjectRoleAllows(role string, action Action) bool {
	value, ok := projectPolicies[action]
	if !ok {
		return false
	}
	for _, allowedRole := range value.AllowedRoles {
		if role == allowedRole {
			return true
		}
	}
	return false
}
