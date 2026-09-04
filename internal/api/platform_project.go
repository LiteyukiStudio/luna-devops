package api

import (
	"context"
	"errors"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/resourceidentifier"
	"gorm.io/gorm"
)

const (
	platformSystemProjectKey        = "platform"
	platformSystemProjectIdentifier = "platform-system"
)

func isSystemProject(project model.Project) bool {
	return strings.TrimSpace(project.SystemKey) != ""
}

func (h *Handlers) ensurePlatformSystemProject(user model.User, ctx context.Context) (model.Project, error) {
	var project model.Project
	if err := h.dbWithContext(ctx).First(&project, "system_key = ?", platformSystemProjectKey).Error; err == nil {
		return h.ensurePlatformSystemProjectBillingOwner(project, user, ctx)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Project{}, err
	}

	project = model.Project{
		ID:                  id.New("prj"),
		Identifier:          platformSystemProjectIdentifier,
		KubernetesNamespace: resourceidentifier.ProjectNamespace(platformSystemProjectIdentifier),
		Name:                "Luna Platform",
		Description:         "Platform-owned applications and probes managed by Luna DevOps.",
		MaxConcurrentBuilds: 1,
		BillingOwnerUserID:  strings.TrimSpace(user.ID),
		SystemKey:           platformSystemProjectKey,
		DeleteStatus:        "active",
	}
	if err := h.dbWithContext(ctx).Create(&project).Error; err != nil {
		var existing model.Project
		if findErr := h.dbWithContext(ctx).First(&existing, "system_key = ?", platformSystemProjectKey).Error; findErr == nil {
			return h.ensurePlatformSystemProjectBillingOwner(existing, user, ctx)
		}
		return model.Project{}, err
	}
	return h.ensurePlatformSystemProjectBillingOwner(project, user, ctx)
}

func (h *Handlers) ensurePlatformSystemProjectBillingOwner(project model.Project, user model.User, ctx context.Context) (model.Project, error) {
	if strings.TrimSpace(project.BillingOwnerUserID) == "" && strings.TrimSpace(user.ID) != "" {
		project.BillingOwnerUserID = user.ID
		if err := h.dbWithContext(ctx).Model(&project).Update("billing_owner_user_id", user.ID).Error; err != nil {
			return model.Project{}, err
		}
	}
	return project, nil
}
