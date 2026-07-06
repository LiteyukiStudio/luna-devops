package api

import (
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	platformSystemProjectKey  = "platform"
	platformSystemProjectSlug = "platform-system"
)

func isSystemProject(project model.Project) bool {
	return strings.TrimSpace(project.SystemKey) != ""
}

func (h *Handlers) ensurePlatformSystemProject(user model.User) (model.Project, error) {
	project := model.Project{
		ID:                  id.New("prj"),
		Slug:                platformSystemProjectSlug,
		Name:                "Liteyuki Platform",
		Description:         "Platform-owned applications and probes managed by Liteyuki DevOps.",
		NamespaceStrategy:   "project",
		MaxConcurrentBuilds: 1,
		BillingOwnerUserID:  strings.TrimSpace(user.ID),
		SystemKey:           platformSystemProjectKey,
		DeleteStatus:        "active",
	}
	err := h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "system_key"}},
		DoNothing: true,
	}).Create(&project).Error
	if err != nil {
		return model.Project{}, err
	}
	err = h.db.First(&project, "system_key = ?", platformSystemProjectKey).Error
	if err != nil && err == gorm.ErrRecordNotFound {
		return model.Project{}, err
	}
	if err != nil {
		return model.Project{}, err
	}
	if strings.TrimSpace(project.BillingOwnerUserID) == "" && strings.TrimSpace(user.ID) != "" {
		project.BillingOwnerUserID = user.ID
		if err := h.db.Model(&project).Update("billing_owner_user_id", user.ID).Error; err != nil {
			return model.Project{}, err
		}
	}
	return project, nil
}
