package projectapi

import "github.com/LiteyukiStudio/devops/internal/authz"

type projectMemberInput struct {
	UserID string `json:"userId"`
	Email  string `json:"email"`
	Role   string `json:"role" binding:"required"`
}

type projectMemberResponse struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	UserID    string `json:"userId"`
	Role      string `json:"role"`
	Email     string `json:"email"`
	Name      string `json:"name"`
}

type projectMemberCandidateResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
}

func normalizeProjectRole(role string) string {
	return authz.NormalizeProjectRole(role)
}
