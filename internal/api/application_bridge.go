package api

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/api/applicationapi"
	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/dependency"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type applicationHost struct {
	handlers *Handlers
}

func (host applicationHost) DBFor(ctx *gin.Context) *gorm.DB { return host.handlers.dbFor(ctx) }

func (host applicationHost) DBWithContext(ctx context.Context) *gorm.DB {
	return host.handlers.dbWithContext(ctx)
}

func (host applicationHost) AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return host.handlers.authorizeProject(ctx, action)
}

func (host applicationHost) EnsureProjectCanMutate(ctx *gin.Context, project model.Project) bool {
	return host.handlers.ensureProjectCanMutate(ctx, project)
}

func (host applicationHost) EnsureBillingAllowsDeployChange(ctx *gin.Context, projectID string) bool {
	return host.handlers.ensureBillingAllowsDeployChange(ctx, projectID)
}

func (host applicationHost) AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	host.handlers.auditWithContext(userID, action, resource, success, message, ctx)
}

func (host applicationHost) SecretStore() secret.Store { return host.handlers.secrets }

func (host applicationHost) PublicBaseURL() string { return host.handlers.config.PublicBaseURL }

func (host applicationHost) NotificationEnqueuer() notification.DeliveryEnqueuer {
	return host.handlers.taskClient
}

func applicationDataVolumeInputsToRoot(inputs []applicationapi.DeploymentTargetDataVolumeInput) []deploymentTargetDataVolumeInput {
	result := make([]deploymentTargetDataVolumeInput, 0, len(inputs))
	for _, input := range inputs {
		converted := deploymentTargetDataVolumeInput{
			LogicalName: input.LogicalName, SourceType: input.SourceType, ProjectVolumeID: input.ProjectVolumeID,
			MountPath: input.MountPath, DevicePath: input.DevicePath, ReadOnly: input.ReadOnly,
		}
		if input.EmptyDir != nil {
			converted.EmptyDir = &deploymentTargetEmptyDirInput{Medium: input.EmptyDir.Medium, SizeLimit: input.EmptyDir.SizeLimit}
		}
		result = append(result, converted)
	}
	return result
}

func applicationDataVolumeInputsFromRoot(inputs []deploymentTargetDataVolumeInput) []applicationapi.DeploymentTargetDataVolumeInput {
	result := make([]applicationapi.DeploymentTargetDataVolumeInput, 0, len(inputs))
	for _, input := range inputs {
		converted := applicationapi.DeploymentTargetDataVolumeInput{
			LogicalName: input.LogicalName, SourceType: input.SourceType, ProjectVolumeID: input.ProjectVolumeID,
			MountPath: input.MountPath, DevicePath: input.DevicePath, ReadOnly: input.ReadOnly,
		}
		if input.EmptyDir != nil {
			converted.EmptyDir = &applicationapi.DeploymentTargetEmptyDirInput{Medium: input.EmptyDir.Medium, SizeLimit: input.EmptyDir.SizeLimit}
		}
		result = append(result, converted)
	}
	return result
}

func applicationVolumeChangesFromRoot(changes deploymentVolumeMountChanges) applicationapi.DeploymentVolumeMountChanges {
	attempted := make([]applicationapi.DeploymentVolumeAuditRecord, 0, len(changes.Attempted))
	for _, record := range changes.Attempted {
		attempted = append(attempted, applicationapi.DeploymentVolumeAuditRecord{
			Action: record.Action, Resource: record.Resource, Message: record.Message,
		})
	}
	return applicationapi.DeploymentVolumeMountChanges{
		Bound: changes.Bound, Unbound: changes.Unbound, HookBindings: changes.HookBindings, Attempted: attempted,
	}
}

func applicationVolumeChangesToRoot(changes applicationapi.DeploymentVolumeMountChanges) deploymentVolumeMountChanges {
	attempted := make([]deploymentVolumeAuditRecord, 0, len(changes.Attempted))
	for _, record := range changes.Attempted {
		attempted = append(attempted, deploymentVolumeAuditRecord{
			Action: record.Action, Resource: record.Resource, Message: record.Message,
		})
	}
	return deploymentVolumeMountChanges{
		Bound: changes.Bound, Unbound: changes.Unbound, HookBindings: changes.HookBindings, Attempted: attempted,
	}
}

func (host applicationHost) SyncDeploymentTargetVolumeMounts(ctx context.Context, tx *gorm.DB, target model.DeploymentTarget, inputs []applicationapi.DeploymentTargetDataVolumeInput) (applicationapi.DeploymentVolumeMountChanges, error) {
	changes, err := syncDeploymentTargetVolumeMounts(ctx, tx, target, applicationDataVolumeInputsToRoot(inputs))
	return applicationVolumeChangesFromRoot(changes), err
}

func (host applicationHost) NextReleaseRevisionFor(tx *gorm.DB, projectID, applicationID, deploymentTargetID string) (int, error) {
	return nextReleaseRevisionFor(tx, projectID, applicationID, deploymentTargetID)
}

func (host applicationHost) AuditDeploymentVolumeMountFailure(ctx context.Context, userID string, changes applicationapi.DeploymentVolumeMountChanges, err error) {
	host.handlers.auditDeploymentVolumeMountFailure(ctx, userID, applicationVolumeChangesToRoot(changes), err)
}

func (host applicationHost) AuditDeploymentVolumeMountChanges(ctx context.Context, userID string, target model.DeploymentTarget, changes applicationapi.DeploymentVolumeMountChanges) {
	host.handlers.auditDeploymentVolumeMountChanges(ctx, userID, target, applicationVolumeChangesToRoot(changes))
}

func (host applicationHost) WriteVolumeError(ctx *gin.Context, err error) { writeVolumeError(ctx, err) }

func (host applicationHost) EnqueueDeployRun(ctx context.Context, release model.Release) bool {
	return host.handlers.enqueueDeployRun(ctx, release)
}

func (host applicationHost) DeploymentTargetVolumeMountsByTarget(ctx context.Context, targets []model.DeploymentTarget) (map[string][]model.DeploymentVolumeMount, error) {
	return host.handlers.deploymentTargetVolumeMountsByTarget(ctx, targets)
}

func (host applicationHost) DeploymentTargetResponseFromModel(target model.DeploymentTarget, mounts []model.DeploymentVolumeMount) any {
	return deploymentTargetResponseFromModel(target, mounts)
}

func (host applicationHost) NormalizePublicStage(value string) (string, bool) {
	return normalizePublicStage(value)
}

func (host applicationHost) WriteDeploymentStageInvalid(ctx *gin.Context, path, detail string) {
	writeDeploymentStageInvalid(ctx, path, detail)
}

func (host applicationHost) NormalizeBuildResourceQuantity(ctx *gin.Context, value, fallbackValue, label string) (string, bool) {
	return normalizeBuildResourceQuantity(ctx, value, fallbackValue, label)
}

func (host applicationHost) RuntimeClusterForProjectUse(ctx *gin.Context, user model.User, projectID, clusterID string) (model.RuntimeCluster, bool) {
	return host.handlers.runtimeClusterForProjectUse(ctx, user, projectID, clusterID)
}

func (host applicationHost) RuntimeProjectNamespace(project model.Project) string {
	return runtimeProjectNamespace(project)
}

func (host applicationHost) NormalizeDataVolumes(ctx *gin.Context, inputs []applicationapi.DeploymentTargetDataVolumeInput) ([]applicationapi.DeploymentTargetDataVolumeInput, bool) {
	normalized, ok := normalizeDataVolumes(ctx, applicationDataVolumeInputsToRoot(inputs))
	return applicationDataVolumeInputsFromRoot(normalized), ok
}

func (host applicationHost) NormalizeRuntimeConfigFilesInput(ctx *gin.Context, value string) (string, bool) {
	return normalizeRuntimeConfigFilesInput(ctx, value)
}

func (host applicationHost) NormalizeRuntimeConfigFilePathInput(ctx *gin.Context, value string) (string, bool) {
	return normalizeRuntimeConfigFilePathInput(ctx, value)
}

func (host applicationHost) IsBuildEnvKey(value string) bool { return isBuildEnvKey(value) }

func (host applicationHost) ObserveDeploymentTargets(ctx context.Context, project model.Project, targets []model.DeploymentTarget) {
	host.handlers.observeDeploymentTargets(ctx, project, targets)
}

func (host applicationHost) RuntimeClusterForDeploymentTarget(ctx *gin.Context, target model.DeploymentTarget) (model.RuntimeCluster, bool) {
	return host.handlers.runtimeClusterForDeploymentTarget(ctx, target)
}

func (host applicationHost) DeploymentTargetNamespace(project model.Project, target model.DeploymentTarget) string {
	return deploymentTargetNamespace(project, target)
}

func (host applicationHost) KubernetesClientForDeploymentTargetObservation(project model.Project, target model.DeploymentTarget, ctx context.Context) (*kubeprovider.Client, string, string) {
	return host.handlers.kubernetesClientForDeploymentTargetObservation(project, target, ctx)
}

func (host applicationHost) DeploymentTargetResourceName(target model.DeploymentTarget) string {
	return deploymentTargetResourceName(target)
}

func (host applicationHost) EnqueueApplicationDelete(ctx context.Context, app model.Application, actorID string, deleteData bool) bool {
	if host.handlers.taskClient == nil {
		return false
	}
	_, err := host.handlers.taskClient.EnqueueApplicationDelete(ctx, tasks.ApplicationDeletePayload{
		ApplicationID: app.ID, ProjectID: app.ProjectID, ActorID: actorID, DeleteData: deleteData,
	})
	return err == nil
}

func (h *Handlers) applicationAPI() *applicationapi.Handler {
	return applicationapi.New(applicationHost{handlers: h})
}

func (h *Handlers) ListAppTemplates(ctx *gin.Context) { h.applicationAPI().ListAppTemplates(ctx) }
func (h *Handlers) GetAppTemplate(ctx *gin.Context)   { h.applicationAPI().GetAppTemplate(ctx) }
func (h *Handlers) InstallAppTemplate(ctx *gin.Context) {
	h.applicationAPI().InstallAppTemplate(ctx)
}
func (h *Handlers) ListApplications(ctx *gin.Context) { h.applicationAPI().ListApplications(ctx) }
func (h *Handlers) CreateApplication(ctx *gin.Context) {
	h.applicationAPI().CreateApplication(ctx)
}
func (h *Handlers) GetApplication(ctx *gin.Context) { h.applicationAPI().GetApplication(ctx) }
func (h *Handlers) UpdateApplication(ctx *gin.Context) {
	h.applicationAPI().UpdateApplication(ctx)
}
func (h *Handlers) DeleteApplication(ctx *gin.Context) {
	h.applicationAPI().DeleteApplication(ctx)
}
func (h *Handlers) PreviewApplicationDeletion(ctx *gin.Context) {
	h.applicationAPI().PreviewApplicationDeletion(ctx)
}
func (h *Handlers) GetApplicationTopology(ctx *gin.Context) {
	h.applicationAPI().GetApplicationTopology(ctx)
}
func (h *Handlers) ListServiceBindings(ctx *gin.Context) {
	h.applicationAPI().ListServiceBindings(ctx)
}
func (h *Handlers) CreateServiceBinding(ctx *gin.Context) {
	h.applicationAPI().CreateServiceBinding(ctx)
}
func (h *Handlers) UpdateServiceBinding(ctx *gin.Context) {
	h.applicationAPI().UpdateServiceBinding(ctx)
}
func (h *Handlers) DeleteServiceBinding(ctx *gin.Context) {
	h.applicationAPI().DeleteServiceBinding(ctx)
}
func (h *Handlers) CheckServiceBinding(ctx *gin.Context) {
	h.applicationAPI().CheckServiceBinding(ctx)
}
func (h *Handlers) ListProjectTopologyEdges(ctx *gin.Context) {
	h.applicationAPI().ListProjectTopologyEdges(ctx)
}
func (h *Handlers) CreateProjectTopologyEdge(ctx *gin.Context) {
	h.applicationAPI().CreateProjectTopologyEdge(ctx)
}
func (h *Handlers) UpdateProjectTopologyEdge(ctx *gin.Context) {
	h.applicationAPI().UpdateProjectTopologyEdge(ctx)
}
func (h *Handlers) DeleteProjectTopologyEdge(ctx *gin.Context) {
	h.applicationAPI().DeleteProjectTopologyEdge(ctx)
}

type appTemplateInstallInput = applicationapi.AppTemplateInstallInput
type appTemplateSummaryResponse = applicationapi.AppTemplateSummaryResponse
type appTemplateValueResponse = applicationapi.AppTemplateValueResponse
type appTemplateResponse = applicationapi.AppTemplateResponse
type templateInstallPlan = applicationapi.TemplateInstallPlan
type applicationDeletionTargetPreview = applicationapi.ApplicationDeletionTargetPreview
type applicationDeletionVolumePreview = applicationapi.ApplicationDeletionVolumePreview
type applicationDeletionPreview = applicationapi.ApplicationDeletionPreview
type applicationInput = applicationapi.ApplicationInput
type applicationDeploymentSummary = applicationapi.ApplicationDeploymentSummary
type applicationDeploymentTargetSummary = applicationapi.ApplicationDeploymentTargetSummary
type applicationListItemResponse = applicationapi.ApplicationListItemResponse
type applicationTopologyTargetResponse = applicationapi.ApplicationTopologyTargetResponse
type applicationTopologyWarningResponse = applicationapi.ApplicationTopologyWarningResponse
type applicationTopologyResponse = applicationapi.ApplicationTopologyResponse
type serviceBindingUsage = applicationapi.ServiceBindingUsage

type appTemplateInstallResponse struct {
	Installation     model.AppTemplateInstallation `json:"installation"`
	Application      model.Application             `json:"application"`
	DeploymentTarget deploymentTargetResponse      `json:"deploymentTarget"`
	Release          *model.Release                `json:"release,omitempty"`
}

const applicationRuntimeNotDeployed = applicationapi.ApplicationRuntimeNotDeployed

var (
	errApplicationIdentifierExists = applicationapi.ErrApplicationIdentifierExists
	applicationIconNames           = applicationapi.ApplicationIconNames()
)

func appTemplateSummaryFrom(template appstore.Template) appTemplateSummaryResponse {
	return applicationapi.AppTemplateSummaryFrom(template)
}

func appTemplateDetailFrom(template appstore.Template) appTemplateResponse {
	return applicationapi.AppTemplateDetailFrom(template)
}

func (h *Handlers) buildTemplateInstallPlan(ctx *gin.Context, user model.User, project model.Project, template appstore.Template, input appTemplateInstallInput) (templateInstallPlan, bool) {
	return h.applicationAPI().BuildTemplateInstallPlan(ctx, user, project, template, input)
}

func appTemplateDeploymentDataVolumes(template appstore.Template, selectedProjectVolume *model.ProjectVolume) []deploymentTargetDataVolumeInput {
	return applicationDataVolumeInputsToRoot(applicationapi.AppTemplateDeploymentDataVolumes(template, selectedProjectVolume))
}

func safeTemplateValues(template appstore.Template, values map[string]string) map[string]string {
	return applicationapi.SafeTemplateValues(template, values)
}

func fallbackTemplateIdentifier(slug, appID string) string {
	return applicationapi.FallbackTemplateIdentifier(slug, appID)
}

func templateApplicationIcon(template appstore.Template) string {
	return applicationapi.TemplateApplicationIcon(template)
}

func shortID(value string) string { return applicationapi.ShortID(value) }

func (h *Handlers) findApplication(ctx *gin.Context) (model.Application, bool) {
	return h.applicationAPI().FindApplication(ctx)
}

func applicationCanMutate(app model.Application) bool {
	return applicationapi.ApplicationCanMutate(app)
}

func (h *Handlers) ensureApplicationIdentifierAvailable(ctx *gin.Context, projectID, identifier, excludeApplicationID string) bool {
	return h.applicationAPI().EnsureApplicationIdentifierAvailable(ctx, projectID, identifier, excludeApplicationID)
}

func createApplicationRecord(db *gorm.DB, application *model.Application) error {
	return applicationapi.CreateApplicationRecord(db, application)
}

func writeApplicationIdentifierConflict(ctx *gin.Context, deleteStatus string) {
	applicationapi.WriteApplicationIdentifierConflict(ctx, deleteStatus)
}

func normalizeBuildConcurrencyPolicy(value string) string {
	return applicationapi.NormalizeBuildConcurrencyPolicy(value)
}

func normalizeApplicationIcon(value string) string {
	return applicationapi.NormalizeApplicationIcon(value)
}

func isApplicationIconReference(value string) bool {
	return applicationapi.IsApplicationIconReference(value)
}

func (h *Handlers) buildApplicationDeletionPreview(ctx context.Context, project model.Project, app model.Application) (applicationDeletionPreview, error) {
	return h.applicationAPI().BuildApplicationDeletionPreview(ctx, project, app)
}

func (h *Handlers) enqueueApplicationDelete(ctx context.Context, app model.Application, actorID string, deleteData bool) bool {
	return h.applicationAPI().EnqueueApplicationDelete(ctx, app, actorID, deleteData)
}

func (h *Handlers) ensureNoIncomingServiceBindings(ctx *gin.Context, projectID, targetApplicationID, targetDeploymentTargetID string) bool {
	return h.applicationAPI().EnsureNoIncomingServiceBindings(ctx, projectID, targetApplicationID, targetDeploymentTargetID)
}

func writeServiceBindingInUse(ctx *gin.Context, usages []serviceBindingUsage) {
	applicationapi.WriteServiceBindingInUse(ctx, usages)
}

func (h *Handlers) emitServiceBindingEvent(ctx context.Context, user model.User, project model.Project, binding model.ServiceBinding, status, severity string) {
	h.applicationAPI().EmitServiceBindingEvent(ctx, user, project, binding, status, severity)
}

func serviceBindingNotificationEvent(user model.User, project model.Project, binding model.ServiceBinding, sourceApplication model.Application, sourceTarget model.DeploymentTarget, links map[string]string, status, severity string) notification.Event {
	return applicationapi.ServiceBindingNotificationEvent(user, project, binding, sourceApplication, sourceTarget, links, status, severity)
}

func (h *Handlers) dependencyService(ctx context.Context) *dependency.Service {
	return h.applicationAPI().DependencyService(ctx)
}

func dependencyListOptions(pagination paginationParams) dependency.ListOptions {
	return applicationapi.DependencyListOptions(pagination)
}
func dependencyPagination(ctx *gin.Context, allowed map[string]bool, fallback string) paginationParams {
	return applicationapi.DependencyPagination(ctx, allowed, fallback)
}

func serviceBindingMutationResponseFor(binding model.ServiceBinding) gin.H {
	return applicationapi.ServiceBindingMutationResponseFor(binding)
}

func deletedServiceBindingMutationResponse(binding model.ServiceBinding) gin.H {
	return applicationapi.DeletedServiceBindingMutationResponse(binding)
}

func dependencyAuditMessage(err error) string { return applicationapi.DependencyAuditMessage(err) }

func writeDependencyError(ctx *gin.Context, err error) {
	applicationapi.WriteDependencyError(ctx, err)
}

func (h *Handlers) applicationListItemsWithRuntime(ctx context.Context, project model.Project, applications []model.Application) ([]applicationListItemResponse, error) {
	return h.applicationAPI().ApplicationListItemsWithRuntime(ctx, project, applications)
}

func summarizeApplicationDeploymentTargets(targets []model.DeploymentTarget) applicationDeploymentSummary {
	return applicationapi.SummarizeApplicationDeploymentTargets(targets)
}

func applicationDeploymentStatusPriority(status string) int {
	return applicationapi.ApplicationDeploymentStatusPriority(status)
}

func applicationDeploymentStagePriority(stage string) int {
	return applicationapi.ApplicationDeploymentStagePriority(stage)
}

func (h *Handlers) topologyClusterForTarget(ctx *gin.Context, user model.User, projectID string, target model.DeploymentTarget) (model.RuntimeCluster, bool) {
	return h.applicationAPI().TopologyClusterForTarget(ctx, user, projectID, target)
}
