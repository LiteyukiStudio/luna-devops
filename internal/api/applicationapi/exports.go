package applicationapi

import (
	"context"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/dependency"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const ApplicationRuntimeNotDeployed = applicationRuntimeNotDeployed

var ErrApplicationIdentifierExists = errApplicationIdentifierExists

type AppTemplateInstallInput = appTemplateInstallInput
type AppTemplateSummaryResponse = appTemplateSummaryResponse
type AppTemplateValueResponse = appTemplateValueResponse
type AppTemplateResponse = appTemplateResponse
type TemplateInstallPlan = templateInstallPlan
type ApplicationDeletionTargetPreview = applicationDeletionTargetPreview
type ApplicationDeletionVolumePreview = applicationDeletionVolumePreview
type ApplicationDeletionPreview = applicationDeletionPreview
type ApplicationInput = applicationInput
type ApplicationDeploymentSummary = applicationDeploymentSummary
type ApplicationDeploymentTargetSummary = applicationDeploymentTargetSummary
type ApplicationListItemResponse = applicationListItemResponse
type ApplicationTopologyTargetResponse = applicationTopologyTargetResponse
type ApplicationTopologyWarningResponse = applicationTopologyWarningResponse
type ApplicationTopologyResponse = applicationTopologyResponse
type ServiceBindingUsage = serviceBindingUsage

func AppTemplateSummaryFrom(template appstore.Template) AppTemplateSummaryResponse {
	return appTemplateSummaryFrom(template)
}

func AppTemplateDetailFrom(template appstore.Template) AppTemplateResponse {
	return appTemplateDetailFrom(template)
}

func (h *Handler) BuildTemplateInstallPlan(ctx *gin.Context, user model.User, project model.Project, template appstore.Template, input AppTemplateInstallInput) (TemplateInstallPlan, bool) {
	return h.buildTemplateInstallPlan(ctx, user, project, template, input)
}

func AppTemplateDeploymentDataVolumes(template appstore.Template, selectedProjectVolume *model.ProjectVolume) []DeploymentTargetDataVolumeInput {
	return appTemplateDeploymentDataVolumes(template, selectedProjectVolume)
}

func SafeTemplateValues(template appstore.Template, values map[string]string) map[string]string {
	return safeTemplateValues(template, values)
}

func FallbackTemplateIdentifier(slug, appID string) string {
	return fallbackTemplateIdentifier(slug, appID)
}

func TemplateApplicationIcon(template appstore.Template) string {
	return templateApplicationIcon(template)
}

func ShortID(value string) string { return shortID(value) }

func (h *Handler) FindApplication(ctx *gin.Context) (model.Application, bool) {
	return h.findApplication(ctx)
}

func ApplicationCanMutate(app model.Application) bool { return applicationCanMutate(app) }

func (h *Handler) EnsureApplicationIdentifierAvailable(ctx *gin.Context, projectID, identifier, excludeApplicationID string) bool {
	return h.ensureApplicationIdentifierAvailable(ctx, projectID, identifier, excludeApplicationID)
}

func CreateApplicationRecord(db *gorm.DB, application *model.Application) error {
	return createApplicationRecord(db, application)
}

func WriteApplicationIdentifierConflict(ctx *gin.Context, deleteStatus string) {
	writeApplicationIdentifierConflict(ctx, deleteStatus)
}

func NormalizeBuildConcurrencyPolicy(value string) string {
	return normalizeBuildConcurrencyPolicy(value)
}

func NormalizeApplicationIcon(value string) string { return normalizeApplicationIcon(value) }

func IsApplicationIconReference(value string) bool { return isApplicationIconReference(value) }

func ApplicationIconNames() []string { return append([]string(nil), applicationIconNames...) }

func (h *Handler) BuildApplicationDeletionPreview(ctx context.Context, project model.Project, app model.Application) (ApplicationDeletionPreview, error) {
	return h.buildApplicationDeletionPreview(ctx, project, app)
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

func (h *Handler) EmitServiceBindingEvent(ctx context.Context, user model.User, project model.Project, binding model.ServiceBinding, status, severity string) {
	h.emitServiceBindingEvent(ctx, user, project, binding, status, severity)
}

func ServiceBindingNotificationEvent(user model.User, project model.Project, binding model.ServiceBinding, sourceApplication model.Application, sourceTarget model.DeploymentTarget, links map[string]string, status, severity string) notification.Event {
	return serviceBindingNotificationEvent(user, project, binding, sourceApplication, sourceTarget, links, status, severity)
}

func (h *Handler) DependencyService(ctx context.Context) *dependency.Service {
	return h.dependencyService(ctx)
}

func DependencyListOptions(pagination paginationParams) dependency.ListOptions {
	return dependencyListOptions(pagination)
}

func DependencyPagination(ctx *gin.Context, allowed map[string]bool, fallback string) transportapi.PaginationParams {
	return dependencyPagination(ctx, allowed, fallback)
}

func ServiceBindingMutationResponseFor(binding model.ServiceBinding) gin.H {
	return serviceBindingMutationResponseFor(binding)
}

func DeletedServiceBindingMutationResponse(binding model.ServiceBinding) gin.H {
	return deletedServiceBindingMutationResponse(binding)
}

func DependencyAuditMessage(err error) string { return dependencyAuditMessage(err) }

func WriteDependencyError(ctx *gin.Context, err error) { writeDependencyError(ctx, err) }

func (h *Handler) ApplicationListItemsWithRuntime(ctx context.Context, project model.Project, applications []model.Application) ([]ApplicationListItemResponse, error) {
	return h.applicationListItemsWithRuntime(ctx, project, applications)
}

func SummarizeApplicationDeploymentTargets(targets []model.DeploymentTarget) ApplicationDeploymentSummary {
	return summarizeApplicationDeploymentTargets(targets)
}

func ApplicationDeploymentStatusPriority(status string) int {
	return applicationDeploymentStatusPriority(status)
}

func ApplicationDeploymentStagePriority(stage string) int {
	return applicationDeploymentStagePriority(stage)
}

func (h *Handler) TopologyClusterForTarget(ctx *gin.Context, user model.User, projectID string, target model.DeploymentTarget) (model.RuntimeCluster, bool) {
	return h.topologyClusterForTarget(ctx, user, projectID, target)
}
