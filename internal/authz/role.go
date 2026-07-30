package authz

const (
	PlatformRoleAdmin = "platform_admin"
	PlatformRoleUser  = "user"
)

const (
	ProjectRoleOwner     = "owner"
	ProjectRoleAdmin     = "admin"
	ProjectRoleDeveloper = "developer"
	ProjectRoleViewer    = "viewer"
)

func IsPlatformAdmin(role string) bool {
	return role == PlatformRoleAdmin
}

func IsPlatformRole(role string) bool {
	switch role {
	case PlatformRoleAdmin, PlatformRoleUser:
		return true
	default:
		return false
	}
}

func IsProjectRole(role string) bool {
	switch role {
	case ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer:
		return true
	default:
		return false
	}
}

func NormalizeProjectRole(role string) string {
	if IsProjectRole(role) {
		return role
	}
	return ProjectRoleViewer
}
