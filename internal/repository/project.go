package repository

import (
	"context"
	"errors"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

var errProjectRepositoryUnavailable = errors.New("project repository is unavailable")

type ProjectRepository struct {
	db *gorm.DB
}

func NewProjectRepository(db *gorm.DB) ProjectRepository {
	return ProjectRepository{db: db}
}

// ProjectRole implements authz.ProjectMembershipReader without embedding
// persistence details in the authorization policy package.
func (r ProjectRepository) ProjectRole(ctx context.Context, userID, projectID string) (string, error) {
	if r.db == nil {
		return "", errProjectRepositoryUnavailable
	}
	var member model.ProjectMember
	if err := r.db.WithContext(ctx).First(&member, "project_id = ? and user_id = ?", projectID, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", authz.ErrProjectMembershipNotFound
		}
		return "", err
	}
	return member.Role, nil
}

func (r ProjectRepository) IDsForUserContext(ctx context.Context, userID string) []string {
	policy, ok := authz.ProjectPolicyForAction(authz.ActionProjectRead)
	if !ok {
		return nil
	}
	var projectIDs []string
	_ = r.db.WithContext(ctx).Model(&model.ProjectMember{}).
		Where("user_id = ? and role in ?", userID, policy.AllowedRoles).
		Pluck("project_id", &projectIDs).Error
	return projectIDs
}

func (r ProjectRepository) HasAnotherOwnerContext(ctx context.Context, projectID, memberID string) bool {
	var count int64
	_ = r.db.WithContext(ctx).Model(&model.ProjectMember{}).
		Where("project_id = ? and role = ? and id <> ?", projectID, authz.ProjectRoleOwner, memberID).
		Count(&count).Error
	return count > 0
}
