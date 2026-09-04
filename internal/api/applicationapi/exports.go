package applicationapi

import (
	"context"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

type AppTemplateInstallInput = appTemplateInstallInput

type ServiceBindingUsage = serviceBindingUsage

func AppTemplateDeploymentDataVolumes(template appstore.Template, selectedProjectVolume *model.ProjectVolume) []DeploymentTargetDataVolumeInput {
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

func NormalizeBuildConcurrencyPolicy(value string) string {
	return normalizeBuildConcurrencyPolicy(value)
}

func (h *Handler) EnqueueApplicationDelete(ctx context.Context, app model.Application, actorID string, deleteData bool) bool {
	return h.enqueueApplicationDelete(ctx, app, actorID, deleteData)
}

func (h *Handler) EnsureNoIncomingServiceBindings(ctx *gin.Context, projectID, targetApplicationID, targetDeploymentTargetID string) bool {
	return h.ensureNoIncomingServiceBindings(ctx, projectID, targetApplicationID, targetDeploymentTargetID)
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
