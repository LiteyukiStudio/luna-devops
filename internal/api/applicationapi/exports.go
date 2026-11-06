package applicationapi

import (
	"context"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AppTemplateInstallInput = appTemplateInstallInput

type ServiceBindingUsage = serviceBindingUsage

func AppTemplateDeploymentDataVolumes(template appstore.Workload, selectedProjectVolume *model.ProjectVolume) []DeploymentTargetDataVolumeInput {
	return appTemplateDeploymentDataVolumes(template, selectedProjectVolume)
}

func TemplateApplicationIcon(template appstore.Template) string {
	return templateApplicationIcon(template)
}

func ShortID(value string) string { return shortID(value) }

func (h *Handler) FindApplication(ctx *gin.Context) (model.Application, bool) {
	return h.findApplication(ctx)
}

func ApplicationCanMutate(app model.Application) bool { return applicationCanMutate(app) }

func WriteApplicationIdentifierConflict(ctx *gin.Context, deleteStatus string) {
	writeApplicationIdentifierConflict(ctx, deleteStatus)
}

func (h *Handler) EnqueueApplicationDelete(ctx context.Context, app model.Application, actorID string, deleteData bool) bool {
	return h.enqueueApplicationDelete(ctx, app, actorID, deleteData)
}

func EnsureNoIncomingServiceBindingsTx(tx *gorm.DB, projectID, targetApplicationID, targetDeploymentTargetID string) error {
	return ensureNoIncomingServiceBindingsTx(tx, projectID, targetApplicationID, targetDeploymentTargetID)
}

func WriteServiceBindingInUseError(ctx *gin.Context, err error) bool {
	return writeServiceBindingInUseError(ctx, err)
}

func WriteServiceBindingInUse(ctx *gin.Context, usages []ServiceBindingUsage) {
	writeServiceBindingInUse(ctx, usages)
}

func DependencyPagination(ctx *gin.Context, allowed map[string]bool, fallback string) transportapi.PaginationParams {
	return dependencyPagination(ctx, allowed, fallback)
}

func ServiceBindingMutationResponseFor(binding model.ServiceBinding) gin.H {
	return serviceBindingMutationResponseFor(binding)
}

func WriteDependencyError(ctx *gin.Context, err error) { writeDependencyError(ctx, err) }
