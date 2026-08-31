package authz

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrProjectPolicyUndefined          = errors.New("project authorization policy is undefined")
	ErrProjectAccessDenied             = errors.New("project access is denied")
	ErrProjectMembershipNotFound       = errors.New("project membership is not found")
	ErrProjectAuthorizationUnavailable = errors.New("project authorization is unavailable")
)

type ProjectMembershipReader interface {
	ProjectRole(ctx context.Context, userID, projectID string) (string, error)
}

type ProjectSubject struct {
	UserID       string
	PlatformRole string
}

type ProjectAccess struct {
	Role          string
	PlatformAdmin bool
}

type ProjectAuthorizer interface {
	AuthorizeProject(ctx context.Context, subject ProjectSubject, projectID string, action Action) (ProjectAccess, error)
}

type projectAuthorizer struct {
	memberships ProjectMembershipReader
}

func NewProjectAuthorizer(memberships ProjectMembershipReader) ProjectAuthorizer {
	return projectAuthorizer{memberships: memberships}
}

func (a projectAuthorizer) AuthorizeProject(
	ctx context.Context,
	subject ProjectSubject,
	projectID string,
	action Action,
) (ProjectAccess, error) {
	_, ok := ProjectPolicyForAction(action)
	if !ok {
		return ProjectAccess{}, fmt.Errorf("%w: %s", ErrProjectPolicyUndefined, action)
	}
	if subject.UserID == "" || projectID == "" {
		return ProjectAccess{}, ErrProjectAccessDenied
	}
	if IsPlatformAdmin(subject.PlatformRole) {
		return ProjectAccess{PlatformAdmin: true}, nil
	}
	if a.memberships == nil {
		return ProjectAccess{}, ErrProjectAuthorizationUnavailable
	}

	role, err := a.memberships.ProjectRole(ctx, subject.UserID, projectID)
	if errors.Is(err, ErrProjectMembershipNotFound) {
		return ProjectAccess{}, ErrProjectAccessDenied
	}
	if err != nil {
		return ProjectAccess{}, fmt.Errorf("%w: %v", ErrProjectAuthorizationUnavailable, err)
	}
	if !ProjectRoleAllows(role, action) {
		return ProjectAccess{}, ErrProjectAccessDenied
	}
	return ProjectAccess{Role: role}, nil
}
