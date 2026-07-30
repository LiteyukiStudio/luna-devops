package repository

import (
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

type ProjectRepository struct {
	db *gorm.DB
}

func NewProjectRepository(db *gorm.DB) ProjectRepository {
	return ProjectRepository{db: db}
}

func (r ProjectRepository) IDsForUser(userID string) []string {
	var projectIDs []string
	_ = r.db.Model(&model.ProjectMember{}).Where("user_id = ?", userID).Pluck("project_id", &projectIDs).Error
	return projectIDs
}

func (r ProjectRepository) UserHasProject(userID, projectID string) bool {
	var count int64
	if err := r.db.Model(&model.ProjectMember{}).Where("user_id = ? and project_id = ?", userID, projectID).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func (r ProjectRepository) HasAnotherOwner(projectID, memberID string) bool {
	var count int64
	_ = r.db.Model(&model.ProjectMember{}).
		Where("project_id = ? and role = ? and id <> ?", projectID, authz.ProjectRoleOwner, memberID).
		Count(&count).Error
	return count > 0
}
