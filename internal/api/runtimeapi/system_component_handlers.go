package runtimeapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	"github.com/LiteyukiStudio/devops/internal/resourceidentifier"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	systemComponentGatewayTrafficProbe = model.SystemComponentGatewayTrafficProbe
)

type systemComponentInstallInput struct {
	ClusterID         string `json:"clusterId"`
	Namespace         string `json:"namespace"`
	Mode              string `json:"mode"`
	Image             string `json:"image"`
	ProvisionAccess   *bool  `json:"provisionAccess"`
	APIBaseURL        string `json:"apiBaseUrl"`
	TraefikMetricsURL string `json:"traefikMetricsUrl"`
}

type systemComponentInstallResponse struct {
	Installation     model.SystemComponentInstallation `json:"installation"`
	Application      model.Application                 `json:"application,omitempty"`
	DeploymentTarget any                               `json:"deploymentTarget,omitempty"`
	Release          *model.Release                    `json:"release,omitempty"`
}

type systemComponentStatusResponse struct {
	Items                      []model.SystemComponentInstallation `json:"items"`
	GatewayTrafficProbeEnabled bool                                `json:"gatewayTrafficProbeEnabled"`
}

func (h *Handlers) ListSystemComponents(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var items []model.SystemComponentInstallation
	query := h.dbFor(ctx).Order("component_id asc, runtime_cluster_id asc")
	if componentID := strings.TrimSpace(ctx.Query("componentId")); componentID != "" {
		query = query.Where("component_id = ?", componentID)
	}
	if clusterID := strings.TrimSpace(ctx.Query("clusterId")); clusterID != "" {
		query = query.Where("runtime_cluster_id = ?", clusterID)
	}
	if err := query.Find(&items).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.observeSystemComponentInstallations(ctx.Request.Context(), items)
	ctx.JSON(http.StatusOK, systemComponentStatusResponse{
		Items:                      items,
		GatewayTrafficProbeEnabled: hasReadySystemComponent(items, systemComponentGatewayTrafficProbe),
	})
}

func (h *Handlers) InstallSystemAppTemplate(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	template, found, err := appstore.Find(ctx.Param("templateId"))
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if !found || strings.TrimSpace(template.Kind) != "system_component" || strings.TrimSpace(template.SystemComponent) == "" {
		writeError(ctx, http.StatusNotFound, "system component template not found")
		return
	}
	var input systemComponentInstallInput
	if !bindJSON(ctx, &input) {
		return
	}
	clusterID := strings.TrimSpace(input.ClusterID)
	if clusterID == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "runtime_cluster.required", "runtime cluster is required")
		return
	}
	var cluster model.RuntimeCluster
	if err := runtimecluster.ActiveScope(h.dbFor(ctx)).First(&cluster, "id = ?", clusterID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !h.host.TaskQueueAvailable() {
		writeError(ctx, http.StatusServiceUnavailable, "task queue is not configured")
		return
	}
	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = "traefik-metrics"
	}
	apiBaseURL := strings.TrimRight(strings.TrimSpace(input.APIBaseURL), "/")
	if apiBaseURL == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "system_component.api_base_url_required", "API base URL is required")
		return
	}
	traefikMetricsURL := strings.TrimSpace(input.TraefikMetricsURL)
	if err := validateOptionalHTTPURL(traefikMetricsURL, "Traefik metrics URL"); err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "system_component.traefik_metrics_url_invalid", err.Error())
		return
	}
	configJSON, err := json.Marshal(map[string]string{"apiBaseUrl": apiBaseURL, "traefikMetricsUrl": traefikMetricsURL, "image": strings.TrimSpace(input.Image)})
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	componentID := strings.TrimSpace(template.SystemComponent)
	reportToken := "lyd_probe_" + randomHex(32)

	platformProject, err := h.ensurePlatformSystemProject(user, ctx.Request.Context())
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	provisionAccess := true
	if input.ProvisionAccess != nil {
		provisionAccess = *input.ProvisionAccess
	}
	plan, ok := h.systemComponentApplicationPlan(ctx, user, platformProject, cluster, template, componentID, mode, string(configJSON), apiBaseURL, traefikMetricsURL, reportToken, strings.TrimSpace(input.Image), provisionAccess)
	if !ok {
		return
	}
	if err := h.persistSystemComponentApplicationPlan(plan, ctx.Request.Context()); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if !h.enqueueDeployRun(ctx.Request.Context(), *plan.Release) {
		message := "平台组件部署任务投递失败"
		_ = h.dbFor(ctx).Model(&model.Release{}).Where("id = ? and status in ?", plan.Release.ID, model.ReleaseActiveStatuses).Updates(map[string]any{"status": "failed", "failure_code": model.ReleaseFailureDispatchFailed, "message": message}).Error
		_ = h.dbFor(ctx).Model(&model.SystemComponentInstallation{}).Where("id = ?", plan.Installation.ID).Updates(map[string]any{"status": "failed", "message": message, "last_error": message}).Error
		writeError(ctx, http.StatusServiceUnavailable, message)
		return
	}
	h.auditWithContext(user.ID, "system_component.install", plan.Installation.ID, true, componentID, ctx.Request.Context())
	ctx.JSON(http.StatusCreated, systemComponentInstallResponse{
		Installation:     plan.Installation,
		Application:      plan.Application,
		DeploymentTarget: h.host.DeploymentTargetResponse(plan.DeploymentTarget),
		Release:          plan.Release,
	})
}

func hasReadySystemComponent(items []model.SystemComponentInstallation, componentID string) bool {
	for _, item := range items {
		if item.ComponentID == componentID && item.RuntimeStatus == observation.StatusReady {
			return true
		}
	}
	return false
}

type systemComponentApplicationPlan struct {
	Installation     model.SystemComponentInstallation
	Application      model.Application
	DeploymentTarget model.DeploymentTarget
	Release          *model.Release
	SecretValue      model.SecretValue
}

func (h *Handlers) systemComponentApplicationPlan(ctx *gin.Context, user model.User, project model.Project, cluster model.RuntimeCluster, template appstore.Template, componentID string, mode string, configJSON string, apiBaseURL string, traefikMetricsURL string, reportToken string, imageOverride string, provisionAccess bool) (systemComponentApplicationPlan, bool) {
	applicationIdentifier := strings.TrimSpace(template.Slug)
	if applicationIdentifier == "" {
		applicationIdentifier = componentID
	}
	applicationName := template.Name
	application := model.Application{
		ID:           id.New("app"),
		ProjectID:    project.ID,
		Identifier:   applicationIdentifier,
		Name:         applicationName,
		Icon:         h.host.TemplateApplicationIcon(template),
		DeleteStatus: "active",
	}
	var existingApp model.Application
	if err := h.dbFor(ctx).First(&existingApp, "project_id = ? and identifier = ?", project.ID, applicationIdentifier).Error; err == nil {
		application = existingApp
		application.Name = applicationName
		application.Icon = h.host.TemplateApplicationIcon(template)
		application.DeleteStatus = "active"
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return systemComponentApplicationPlan{}, false
	}

	clusterSuffix := h.host.ShortID(cluster.ID)
	systemStage := "sys-" + clusterSuffix
	// provisionAccess is a trusted internal plan input. The Worker revalidates the
	// installation record before provisioning the dedicated identity and RBAC.
	serviceAccountName := ""
	automountToken := ""
	if provisionAccess {
		serviceAccountName = "luna-gateway-traffic-probe"
		automountToken = "true"
	}
	defaultTargetName := "cluster-" + clusterSuffix
	defaultKubernetesName := resourceidentifier.DeploymentTargetName(applicationIdentifier, "system-"+clusterSuffix)
	defaultServicePorts := model.EncodeDeploymentServicePorts([]model.DeploymentServicePort{{Name: "metrics", Port: fallbackInt(template.ServicePort, 9090)}}, fallbackInt(template.ServicePort, 9090))
	defaultImageRef := firstNonEmpty(template.Image, "liteyukistudio/devops-gateway-traffic-probe:nightly")
	target := model.DeploymentTarget{
		ID:                       id.New("dplt"),
		ProjectID:                project.ID,
		ApplicationID:            application.ID,
		Name:                     defaultTargetName,
		Stage:                    systemStage,
		KubernetesName:           defaultKubernetesName,
		ClusterID:                cluster.ID,
		WorkloadType:             "Deployment",
		Replicas:                 1,
		CPURequest:               firstNonEmpty(template.DefaultCPU, "100m"),
		MemoryRequest:            firstNonEmpty(template.DefaultMemory, "128Mi"),
		AllowPrivilegeEscalation: "false",
		ServiceType:              "ClusterIP",
		AutoScalingMinReplicas:   1,
		AutoScalingMaxReplicas:   1,
		ServicePorts:             defaultServicePorts,
		SourceType:               "image",
		BuildDefinitionMode:      "repository_dockerfile",
		BuildTemplateValues:      "{}",
		DockerfilePath:           "Dockerfile",
		BuildContext:             ".",
		// Users may override the probe image; always pull so mutable tags such as
		// nightly are re-resolved instead of served from a stale node cache.
		ImageRef:                     firstNonEmpty(imageOverride, defaultImageRef),
		ImagePullPolicy:              "Always",
		BuildCPURequest:              model.DefaultBuildCPURequest,
		BuildMemoryRequest:           model.DefaultBuildMemoryRequest,
		BuildTimeoutSeconds:          1800,
		BuildHooksEnabled:            true,
		BuildConcurrencyPolicy:       model.BuildConcurrencyQueue,
		EnvVars:                      systemComponentProbeEnv(cluster, componentID, mode, apiBaseURL, traefikMetricsURL),
		DataMountPath:                "/data",
		Enabled:                      true,
		DeleteStatus:                 "active",
		ServiceAccountName:           serviceAccountName,
		AutomountServiceAccountToken: automountToken,
		CreatedBy:                    user.ID,
	}
	var existingTarget model.DeploymentTarget
	if err := h.dbFor(ctx).First(&existingTarget, "project_id = ? and application_id = ? and (stage = ? or name = ?) and deleted_at is null", project.ID, application.ID, systemStage, defaultTargetName).Error; err == nil {
		target = existingTarget
		target.ProjectID = project.ID
		target.ApplicationID = application.ID
		target.Name = firstNonEmpty(existingTarget.Name, defaultTargetName)
		target.Stage = firstNonEmpty(existingTarget.Stage, systemStage)
		target.KubernetesName = firstNonEmpty(existingTarget.KubernetesName, defaultKubernetesName)
		target.ClusterID = cluster.ID
		target.WorkloadType = firstNonEmpty(existingTarget.WorkloadType, "Deployment")
		if target.Replicas < 1 {
			target.Replicas = 1
		}
		target.CPURequest = firstNonEmpty(existingTarget.CPURequest, template.DefaultCPU, "100m")
		target.MemoryRequest = firstNonEmpty(existingTarget.MemoryRequest, template.DefaultMemory, "128Mi")
		target.AllowPrivilegeEscalation = firstNonEmpty(existingTarget.AllowPrivilegeEscalation, "false")
		target.ServiceType = firstNonEmpty(existingTarget.ServiceType, "ClusterIP")
		if target.AutoScalingMinReplicas < 1 {
			target.AutoScalingMinReplicas = 1
		}
		if target.AutoScalingMaxReplicas < target.AutoScalingMinReplicas {
			target.AutoScalingMaxReplicas = target.AutoScalingMinReplicas
		}
		target.ServicePorts = defaultServicePorts
		target.SourceType = "image"
		target.BuildDefinitionMode = firstNonEmpty(existingTarget.BuildDefinitionMode, "repository_dockerfile")
		target.BuildTemplateValues = firstNonEmpty(existingTarget.BuildTemplateValues, "{}")
		target.DockerfilePath = firstNonEmpty(existingTarget.DockerfilePath, "Dockerfile")
		target.BuildContext = firstNonEmpty(existingTarget.BuildContext, ".")
		target.ImageRef = firstNonEmpty(imageOverride, existingTarget.ImageRef, defaultImageRef)
		target.ImagePullPolicy = "Always"
		target.BuildCPURequest = firstNonEmpty(existingTarget.BuildCPURequest, model.DefaultBuildCPURequest)
		target.BuildMemoryRequest = firstNonEmpty(existingTarget.BuildMemoryRequest, model.DefaultBuildMemoryRequest)
		if target.BuildTimeoutSeconds <= 0 {
			target.BuildTimeoutSeconds = 1800
		}
		target.BuildConcurrencyPolicy = firstNonEmpty(existingTarget.BuildConcurrencyPolicy, model.BuildConcurrencyQueue)
		target.EnvVars = systemComponentProbeEnv(cluster, componentID, mode, apiBaseURL, traefikMetricsURL)
		target.DataMountPath = firstNonEmpty(existingTarget.DataMountPath, "/data")
		target.Enabled = true
		target.DeleteStatus = "active"
		target.DeleteMessage = ""
		target.DeleteCode = ""
		target.ServiceAccountName = serviceAccountName
		target.AutomountServiceAccountToken = automountToken
		target.CreatedBy = firstNonEmpty(existingTarget.CreatedBy, user.ID)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return systemComponentApplicationPlan{}, false
	}
	secretValue := model.SecretValue{
		ID:        id.New("sec"),
		CipherRef: h.secrets.Encrypt(reportToken),
		CreatedBy: user.ID,
		Resource:  "system_component:" + componentID + ":" + cluster.ID + ":report_token",
	}
	if secretValue.CipherRef == "" {
		writeError(ctx, http.StatusInternalServerError, "密钥加密失败")
		return systemComponentApplicationPlan{}, false
	}
	secretRefs, err := json.Marshal(map[string]string{"REPORT_TOKEN": "secret-id:" + secretValue.ID})
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return systemComponentApplicationPlan{}, false
	}
	target.SecretRefs = string(secretRefs)

	release := &model.Release{
		ID:                 id.New("rel"),
		ProjectID:          project.ID,
		ApplicationID:      application.ID,
		DeploymentTargetID: target.ID,
		ImageRef:           target.ImageRef,
		ForceImagePull:     true,
		Type:               "deploy",
		Status:             "pending",
		Message:            "system component install",
		CreatedBy:          user.ID,
	}
	installation := model.SystemComponentInstallation{
		ID:                 id.New("scmp"),
		ComponentID:        componentID,
		ComponentVersion:   template.Version,
		RuntimeClusterID:   cluster.ID,
		ProjectID:          project.ID,
		ApplicationID:      application.ID,
		DeploymentTargetID: target.ID,
		ReleaseID:          release.ID,
		Namespace:          strings.TrimSpace(project.KubernetesNamespace),
		Status:             "deploying",
		RuntimeStatus:      observation.StatusProgressing,
		Message:            "system component application deploy queued",
		ControllerType:     firstNonEmpty(cluster.GatewayControllerType, "traefik"),
		Mode:               mode,
		Config:             configJSON,
		ReportTokenHash:    hashToken(reportToken),
		InstalledBy:        user.ID,
	}
	return systemComponentApplicationPlan{
		Installation:     installation,
		Application:      application,
		DeploymentTarget: target,
		Release:          release,
		SecretValue:      secretValue,
	}, true
}

func (h *Handlers) persistSystemComponentApplicationPlan(plan systemComponentApplicationPlan, ctx context.Context) error {
	return h.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&plan.Application).Error; err != nil {
			return err
		}
		if err := tx.Save(&plan.DeploymentTarget).Error; err != nil {
			return err
		}
		revision, err := h.host.NextReleaseRevision(tx, plan.Release.ProjectID, plan.Release.ApplicationID, plan.Release.DeploymentTargetID)
		if err != nil {
			return err
		}
		plan.Release.Revision = revision
		if err := tx.Create(plan.Release).Error; err != nil {
			return err
		}
		if err := tx.Create(&plan.SecretValue).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "component_id"}, {Name: "runtime_cluster_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"component_version":    plan.Installation.ComponentVersion,
				"project_id":           plan.Installation.ProjectID,
				"application_id":       plan.Installation.ApplicationID,
				"deployment_target_id": plan.Installation.DeploymentTargetID,
				"release_id":           plan.Release.ID,
				"namespace":            plan.Installation.Namespace,
				"status":               "deploying",
				"message":              plan.Installation.Message,
				"controller_type":      plan.Installation.ControllerType,
				"mode":                 plan.Installation.Mode,
				"config":               plan.Installation.Config,
				"report_token_hash":    plan.Installation.ReportTokenHash,
				"last_error":           "",
				"installed_by":         plan.Installation.InstalledBy,
			}),
		}).Create(&plan.Installation).Error
	})
}

func systemComponentProbeEnv(cluster model.RuntimeCluster, componentID string, mode string, apiBaseURL string, traefikMetricsURL string) string {
	gatewayNamespace := firstNonEmpty(cluster.GatewayNamespace, "kube-system")
	values := map[string]string{
		"API_BASE_URL":        strings.TrimRight(apiBaseURL, "/"),
		"COMPONENT_ID":        componentID,
		"RUNTIME_CLUSTER_ID":  cluster.ID,
		"CONTROLLER_TYPE":     firstNonEmpty(cluster.GatewayControllerType, "traefik"),
		"MODE":                firstNonEmpty(mode, "traefik-metrics"),
		"GATEWAY_NAMESPACE":   gatewayNamespace,
		"TRAEFIK_METRICS_URL": firstNonEmpty(traefikMetricsURL, "http://traefik."+gatewayNamespace+".svc.cluster.local:9100/metrics"),
		"PROBE_ADDR":          ":9090",
		"SCRAPE_INTERVAL":     "60s",
	}
	content, err := json.Marshal(values)
	if err != nil {
		return "{}"
	}
	return string(content)
}

func validateOptionalHTTPURL(value string, label string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s must be a valid URL", label)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", label)
	}
	return nil
}

func (h *Handlers) systemComponentForBearerToken(token string, componentID string, ctx context.Context) (model.SystemComponentInstallation, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return model.SystemComponentInstallation{}, false
	}
	var installation model.SystemComponentInstallation
	err := h.dbWithContext(ctx).First(&installation, "component_id = ? and report_token_hash = ?", componentID, hashToken(token)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.SystemComponentInstallation{}, false
	}
	return installation, err == nil
}
