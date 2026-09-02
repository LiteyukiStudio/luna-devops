package api

import (
	"context"
	"errors"

	"github.com/LiteyukiStudio/devops/internal/api/gatewayapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type gatewayHost struct {
	handlers *Handlers
}

func (host gatewayHost) DBFor(ctx *gin.Context) *gorm.DB {
	return host.handlers.dbFor(ctx)
}

func (host gatewayHost) DBWithContext(ctx context.Context) *gorm.DB {
	return host.handlers.dbWithContext(ctx)
}

func (host gatewayHost) AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return host.handlers.authorizeProject(ctx, action)
}

func (host gatewayHost) EnsureProjectCanMutate(ctx *gin.Context, project model.Project) bool {
	return host.handlers.ensureProjectCanMutate(ctx, project)
}

func (host gatewayHost) EnsureGatewayRouteCanMutate(ctx *gin.Context, route model.GatewayRoute) bool {
	return host.handlers.ensureGatewayRouteCanMutate(ctx, route)
}

func (gatewayHost) DeleteStatusCanStart(status string) bool {
	return deleteStatusCanStart(status)
}

func (gatewayHost) ResourceDeleteAlreadyStarted(err error) bool {
	return errors.Is(err, errResourceDeleteAlreadyStarted)
}

func (gatewayHost) MarkResourceDeleting(tx *gorm.DB, resource any, resourceID string) error {
	return markResourceDeleting(tx, resource, resourceID)
}

func (gatewayHost) MarkResourceDeleteFailed(db *gorm.DB, resource any, resourceID, message string) error {
	return markResourceDeleteFailed(db, resource, resourceID, message)
}

func (host gatewayHost) EnqueueResourceCleanup(ctx context.Context, resourceType, resourceID, projectID, actorID string) bool {
	return host.handlers.enqueueResourceCleanup(ctx, resourceType, resourceID, projectID, actorID)
}

func (host gatewayHost) EnqueueGatewayApply(ctx context.Context, route model.GatewayRoute, actorID string) bool {
	if host.handlers.taskClient == nil {
		return false
	}
	_, err := host.handlers.taskClient.EnqueueGatewayApply(ctx, tasks.GatewayApplyPayload{
		GatewayRouteID:          route.ID,
		ProjectID:               route.ProjectID,
		ActorID:                 actorID,
		RouteUpdatedAtUnixMicro: route.UpdatedAt.UTC().UnixMicro(),
	})
	return err == nil
}

func (host gatewayHost) AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	host.handlers.auditWithContext(userID, action, resource, success, message, ctx)
}

func (host gatewayHost) ProjectMemberActionAllowed(ctx *gin.Context, projectID, userID string, action authz.Action) (bool, bool) {
	return host.handlers.projectMemberActionAllowed(ctx, projectID, userID, action)
}

func (host gatewayHost) ConfigValue(key string) string {
	values := host.handlers.configs.get([]string{key})
	return values[key]
}

func (host gatewayHost) SecretStore() secret.Store { return host.handlers.secrets }

func (gatewayHost) NormalizeStage(value string) string { return normalizeStage(value) }

func (gatewayHost) NormalizeGatewayDomainSuffixValue(value string) string {
	return normalizeGatewayDomainSuffixValue(value)
}

func (gatewayHost) DecodeGatewayDomainSuffixes(raw, legacyValue, fallbackValue string) []string {
	return decodeGatewayDomainSuffixes(raw, legacyValue, fallbackValue)
}

func (gatewayHost) NormalizeGatewayPublicScheme(value string) string {
	return normalizeGatewayPublicScheme(value)
}

func (gatewayHost) NormalizePort(value, fallbackValue int) int {
	return normalizePort(value, fallbackValue)
}

func (gatewayHost) ApplicationCanMutate(application model.Application) bool {
	return applicationCanMutate(application)
}

func (gatewayHost) RuntimeProjectNamespace(project model.Project) string {
	return runtimeProjectNamespace(project)
}

func (gatewayHost) DeploymentTargetResourceName(target model.DeploymentTarget) string {
	return deploymentTargetResourceName(target)
}

func (gatewayHost) DeploymentTargetEnvironmentProfile(target model.DeploymentTarget) model.Environment {
	return deploymentTargetEnvironmentProfile(target)
}

func (host gatewayHost) RuntimeClusterForDeploymentTarget(ctx context.Context, target model.DeploymentTarget) (model.RuntimeCluster, error) {
	return runtimeClusterForDeploymentTargetDB(host.handlers.dbWithContext(ctx), target)
}

func (h *Handlers) gatewayAPI() *gatewayapi.Handler {
	return gatewayapi.New(gatewayHost{handlers: h})
}

func (h *Handlers) ListGatewayRoutes(ctx *gin.Context)  { h.gatewayAPI().ListGatewayRoutes(ctx) }
func (h *Handlers) GetGatewayRoute(ctx *gin.Context)    { h.gatewayAPI().GetGatewayRoute(ctx) }
func (h *Handlers) CreateGatewayRoute(ctx *gin.Context) { h.gatewayAPI().CreateGatewayRoute(ctx) }
func (h *Handlers) UpdateGatewayRoute(ctx *gin.Context) { h.gatewayAPI().UpdateGatewayRoute(ctx) }
func (h *Handlers) DeleteGatewayRoute(ctx *gin.Context) { h.gatewayAPI().DeleteGatewayRoute(ctx) }
func (h *Handlers) CheckGatewayDomain(ctx *gin.Context) { h.gatewayAPI().CheckGatewayDomain(ctx) }

type gatewayRouteInput = gatewayapi.GatewayRouteInput
type gatewayRouteAdvancedConfig = gatewayapi.GatewayRouteAdvancedConfig

const gatewayObservationTimeout = gatewayapi.GatewayObservationTimeout

var gatewayHostSegmentPattern = gatewayapi.GatewayHostSegmentPattern

func gatewayHostSegment(value string) string { return gatewayapi.GatewayHostSegment(value) }

func parseGatewayHeaderMap(value string, allowPrivileged bool) (map[string]string, error) {
	return gatewayapi.ParseGatewayHeaderMap(value, allowPrivileged)
}

func parseGatewayKeyValueMap(value string) (map[string]string, error) {
	return gatewayapi.ParseGatewayKeyValueMap(value)
}

func compactGatewayKeyValueMap(values map[string]string) map[string]string {
	return gatewayapi.CompactGatewayKeyValueMap(values)
}

func encodeGatewayHeaderMap(values map[string]string) string {
	return gatewayapi.EncodeGatewayHeaderMap(values)
}

func looksLikeSecretValue(value string) bool { return gatewayapi.LooksLikeSecretValue(value) }

func normalizeHTTPRoutePathMatchType(value string) string {
	return gatewayapi.NormalizeHTTPRoutePathMatchType(value)
}

func normalizeBackendWeight(value int) int { return gatewayapi.NormalizeBackendWeight(value) }

func validateGatewayRouteFilterJSON(label, value string) error {
	return gatewayapi.ValidateGatewayRouteFilterJSON(label, value)
}

func unavailableGatewayRoute(route model.GatewayRoute, code string) model.GatewayRoute {
	return gatewayapi.UnavailableGatewayRoute(route, code)
}

func gatewayRouteStatusFromSummary(summary string) string {
	return gatewayapi.GatewayRouteStatusFromSummary(summary)
}

func gatewayRouteConditions(conditions []kubeprovider.RouteConditionSnapshot) []model.RouteCondition {
	return gatewayapi.GatewayRouteConditions(conditions)
}

func observeGatewayRouteDNS(ctx context.Context, route model.GatewayRoute) string {
	return gatewayapi.ObserveGatewayRouteDNS(ctx, route)
}

func observeGatewayRouteCertificate(
	ctx context.Context,
	client *kubeprovider.Client,
	cluster model.RuntimeCluster,
	projectNamespace string,
	resourceName string,
	route *model.GatewayRoute,
) {
	gatewayapi.ObserveGatewayRouteCertificate(ctx, client, cluster, projectNamespace, resourceName, route)
}

func gatewayRouteCertificateFallback(route model.GatewayRoute) string {
	return gatewayapi.GatewayRouteCertificateFallback(route)
}

func gatewayRouteCertificateIssuerKind(cluster model.RuntimeCluster) string {
	return gatewayapi.GatewayRouteCertificateIssuerKind(cluster)
}

func gatewayRouteAccessURL(route model.GatewayRoute, scheme string, publicPort int) string {
	return gatewayapi.GatewayRouteAccessURL(route, scheme, publicPort)
}

func shouldShowGatewayPublicPort(scheme string, publicPort int) bool {
	return gatewayapi.ShouldShowGatewayPublicPort(scheme, publicPort)
}

func normalizeGatewayRouteSectionName(value string, cluster model.RuntimeCluster) (string, error) {
	return gatewayapi.NormalizeGatewayRouteSectionName(value, cluster)
}

func gatewayAdvancedConfigRequiresProjectAdmin(config gatewayRouteAdvancedConfig) bool {
	return gatewayapi.GatewayAdvancedConfigRequiresProjectAdmin(config)
}

func gatewayAdvancedConfigPresent(config gatewayRouteAdvancedConfig) bool {
	return gatewayapi.GatewayAdvancedConfigPresent(config)
}

func deploymentTargetServicePort(target model.DeploymentTarget) int {
	return gatewayapi.DeploymentTargetServicePort(target)
}

func deploymentTargetHasServicePort(target model.DeploymentTarget, port int) bool {
	return gatewayapi.DeploymentTargetHasServicePort(target, port)
}

func gatewayRouteInputEnabled(value *bool) bool { return gatewayapi.GatewayRouteInputEnabled(value) }

func normalizeTLSMode(value string) string { return gatewayapi.NormalizeTLSMode(value) }

func (h *Handlers) gatewayClusterForDomainCheck(ctx *gin.Context) model.RuntimeCluster {
	return h.gatewayAPI().GatewayClusterForDomainCheck(ctx)
}

func (h *Handlers) defaultGatewayHost(project model.Project, stage, applicationIdentifier string, cluster model.RuntimeCluster, domainSuffix string, ctx context.Context) string {
	return h.gatewayAPI().DefaultGatewayHost(project, stage, applicationIdentifier, cluster, domainSuffix, ctx)
}

func (h *Handlers) gatewayCNAMETarget(cluster model.RuntimeCluster, domainSuffix string) string {
	return h.gatewayAPI().GatewayCNAMETarget(cluster, domainSuffix)
}

func (h *Handlers) normalizeGatewayHost(value string, cluster model.RuntimeCluster, domainSuffix string) string {
	return h.gatewayAPI().NormalizeGatewayHost(value, cluster, domainSuffix)
}

func (h *Handlers) gatewayRootDomain(cluster model.RuntimeCluster) string {
	return h.gatewayAPI().GatewayRootDomain(cluster)
}

func (h *Handlers) gatewayDomainSuffix(cluster model.RuntimeCluster, selected string) string {
	return h.gatewayAPI().GatewayDomainSuffix(cluster, selected)
}

func (h *Handlers) gatewayDomainSuffixes(cluster model.RuntimeCluster) []string {
	return h.gatewayAPI().GatewayDomainSuffixes(cluster)
}

func (h *Handlers) gatewayRouteDomainSuffix(ctx *gin.Context, selected string, cluster model.RuntimeCluster) (string, bool) {
	return h.gatewayAPI().GatewayRouteDomainSuffix(ctx, selected, cluster)
}

func (h *Handlers) gatewayPublicScheme(cluster model.RuntimeCluster) string {
	return h.gatewayAPI().GatewayPublicScheme(cluster)
}

func (h *Handlers) gatewayPublicPort(cluster model.RuntimeCluster) int {
	return h.gatewayAPI().GatewayPublicPort(cluster)
}

func (h *Handlers) legacyGatewayRootDomain() string {
	return h.gatewayAPI().LegacyGatewayRootDomain()
}

func (h *Handlers) gatewayRouteWithAccessURL(route model.GatewayRoute, ctx context.Context) model.GatewayRoute {
	return h.gatewayAPI().GatewayRouteWithAccessURL(route, ctx)
}

func (h *Handlers) gatewayRoutesWithAccessURL(routes []model.GatewayRoute, ctx context.Context) []model.GatewayRoute {
	return h.gatewayAPI().GatewayRoutesWithAccessURL(routes, ctx)
}

func (h *Handlers) gatewayHostExists(host, routeID string, ctx context.Context) bool {
	return h.gatewayAPI().GatewayHostExists(host, routeID, ctx)
}

func (h *Handlers) observeGatewayRoutes(ctx context.Context, routes []model.GatewayRoute) []model.GatewayRoute {
	return h.gatewayAPI().ObserveGatewayRoutes(ctx, routes)
}

func (h *Handlers) observeGatewayRoute(ctx context.Context, route model.GatewayRoute) model.GatewayRoute {
	return h.gatewayAPI().ObserveGatewayRoute(ctx, route)
}

func (h *Handlers) gatewayRouteFromInput(ctx *gin.Context, project model.Project, user model.User, creatorID string, input gatewayRouteInput, routeID string) (model.GatewayRoute, bool) {
	return h.gatewayAPI().GatewayRouteFromInput(ctx, project, user, creatorID, input, routeID)
}

func (h *Handlers) ensureGatewayRouteBackendAvailable(ctx *gin.Context, route model.GatewayRoute) bool {
	return h.gatewayAPI().EnsureGatewayRouteBackendAvailable(ctx, route)
}

func (h *Handlers) gatewayRouteAdvancedConfig(ctx *gin.Context, projectID string, user model.User, cluster model.RuntimeCluster, input gatewayRouteInput) (gatewayRouteAdvancedConfig, bool) {
	return h.gatewayAPI().GatewayRouteAdvancedConfig(ctx, projectID, user, cluster, input)
}

func (h *Handlers) gatewayRouteTargetContext(ctx *gin.Context, projectID string, input gatewayRouteInput) (model.DeploymentTarget, model.Application, model.Environment, model.RuntimeCluster, bool) {
	return h.gatewayAPI().GatewayRouteTargetContext(ctx, projectID, input)
}

func (h *Handlers) runtimeClusterForGatewayRoute(route model.GatewayRoute, ctx context.Context) (model.RuntimeCluster, error) {
	return h.gatewayAPI().RuntimeClusterForGatewayRoute(route, ctx)
}

func (h *Handlers) runtimeClusterForDeploymentTargetValue(target model.DeploymentTarget, ctx context.Context) (model.RuntimeCluster, error) {
	return h.gatewayAPI().RuntimeClusterForDeploymentTargetValue(target, ctx)
}

func (h *Handlers) findGatewayRoute(ctx *gin.Context) (model.GatewayRoute, bool) {
	return h.gatewayAPI().FindGatewayRoute(ctx)
}

func (h *Handlers) enqueueGatewayApply(ctx context.Context, route model.GatewayRoute, actorID string) bool {
	return h.gatewayAPI().EnqueueGatewayApply(ctx, route, actorID)
}

func (h *Handlers) configValue(key string) string { return h.gatewayAPI().ConfigValue(key) }
