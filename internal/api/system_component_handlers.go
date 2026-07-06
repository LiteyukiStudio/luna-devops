package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	systemComponentGatewayTrafficProbe = "gateway-traffic-probe"
)

type systemComponentInstallInput struct {
	ClusterID  string `json:"clusterId"`
	Namespace  string `json:"namespace"`
	Mode       string `json:"mode"`
	APIBaseURL string `json:"apiBaseUrl"`
}

type systemComponentInstallResponse struct {
	Installation     model.SystemComponentInstallation `json:"installation"`
	Application      model.Application                 `json:"application,omitempty"`
	DeploymentTarget deploymentTargetResponse          `json:"deploymentTarget,omitempty"`
	Release          *model.Release                    `json:"release,omitempty"`
}

type systemComponentStatusResponse struct {
	Items                      []model.SystemComponentInstallation `json:"items"`
	GatewayTrafficProbeEnabled bool                                `json:"gatewayTrafficProbeEnabled"`
}

func (h *Handlers) ListSystemComponents(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != "platform_admin" {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var items []model.SystemComponentInstallation
	query := h.db.Order("component_id asc, runtime_cluster_id asc")
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
	if user.Role != "platform_admin" {
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
	if err := h.db.First(&cluster, "id = ?", clusterID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if cluster.Type != "kubernetes" && cluster.Type != "k3s" {
		writeErrorCode(ctx, http.StatusBadRequest, "runtime_cluster.unsupported", "only kubernetes/k3s runtime clusters are supported")
		return
	}
	if h.taskClient == nil {
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
	configJSON, err := json.Marshal(map[string]string{"apiBaseUrl": apiBaseURL})
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	componentID := strings.TrimSpace(template.SystemComponent)
	reportToken := "lyd_probe_" + randomHex(32)

	platformProject, err := h.ensurePlatformSystemProject(user)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	plan, ok := h.systemComponentApplicationPlan(ctx, user, platformProject, cluster, template, componentID, mode, string(configJSON), apiBaseURL, reportToken)
	if !ok {
		return
	}
	if err := h.persistSystemComponentApplicationPlan(plan); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if !h.enqueueDeployRun(ctx.Request.Context(), *plan.Release) {
		message := "平台组件部署任务投递失败"
		_ = h.db.Model(&model.Release{}).Where("id = ?", plan.Release.ID).Updates(map[string]any{"status": "failed", "message": message}).Error
		_ = h.db.Model(&model.SystemComponentInstallation{}).Where("id = ?", plan.Installation.ID).Updates(map[string]any{"status": "failed", "message": message, "last_error": message}).Error
		writeError(ctx, http.StatusServiceUnavailable, message)
		return
	}
	h.audit(user.ID, "system_component.install", plan.Installation.ID, true, componentID)
	ctx.JSON(http.StatusCreated, systemComponentInstallResponse{
		Installation:     plan.Installation,
		Application:      plan.Application,
		DeploymentTarget: deploymentTargetResponseFromModel(plan.DeploymentTarget),
		Release:          plan.Release,
	})
}

func hasReadySystemComponent(items []model.SystemComponentInstallation, componentID string) bool {
	for _, item := range items {
		if item.ComponentID == componentID && (item.Status == "ready" || item.Status == "deployed") {
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

func (h *Handlers) systemComponentApplicationPlan(ctx *gin.Context, user model.User, project model.Project, cluster model.RuntimeCluster, template appstore.Template, componentID string, mode string, configJSON string, apiBaseURL string, reportToken string) (systemComponentApplicationPlan, bool) {
	applicationSlug := strings.TrimSpace(template.Slug)
	if applicationSlug == "" {
		applicationSlug = componentID
	}
	applicationName := template.Name
	application := model.Application{
		ID:                id.New("app"),
		ProjectID:         project.ID,
		Slug:              applicationSlug,
		Name:              applicationName,
		Icon:              templateApplicationIcon(template),
		DeleteStatus:      "active",
		DataRetentionMode: "retain",
	}
	var existingApp model.Application
	if err := h.db.First(&existingApp, "project_id = ? and slug = ?", project.ID, applicationSlug).Error; err == nil {
		application = existingApp
		application.Name = applicationName
		application.Icon = templateApplicationIcon(template)
		application.DeleteStatus = "active"
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return systemComponentApplicationPlan{}, false
	}

	target := model.DeploymentTarget{
		ID:                           id.New("dplt"),
		ProjectID:                    project.ID,
		ApplicationID:                application.ID,
		EnvironmentID:                "",
		Name:                         "cluster-" + shortID(cluster.ID),
		Stage:                        "system",
		ClusterID:                    cluster.ID,
		Replicas:                     1,
		CPURequest:                   firstNonEmpty(template.DefaultCPU, "100m"),
		MemoryRequest:                firstNonEmpty(template.DefaultMemory, "128Mi"),
		ServicePort:                  fallbackInt(template.ServicePort, 9090),
		ServicePorts:                 model.EncodeDeploymentServicePorts([]model.DeploymentServicePort{{Name: "metrics", Port: fallbackInt(template.ServicePort, 9090)}}, fallbackInt(template.ServicePort, 9090)),
		SourceType:                   "image",
		ImageRef:                     firstNonEmpty(template.Image, "liteyukistudio/devops-gateway-traffic-probe:nightly"),
		ImagePullPolicy:              "IfNotPresent",
		BuildCPURequest:              defaultBuildCPURequest,
		BuildMemoryRequest:           defaultBuildMemoryRequest,
		BuildTimeoutSeconds:          defaultBuildTimeoutSeconds,
		ConcurrencyPolicy:            "queue",
		EnvVars:                      systemComponentProbeEnv(cluster, componentID, mode, apiBaseURL),
		DataRetentionEnabled:         false,
		Enabled:                      true,
		DeleteStatus:                 "active",
		ServiceAccountName:           "liteyuki-gateway-traffic-probe",
		AutomountServiceAccountToken: "true",
		CreatedBy:                    user.ID,
	}
	var existingTarget model.DeploymentTarget
	if err := h.db.First(&existingTarget, "project_id = ? and application_id = ? and name = ? and deleted_at is null", project.ID, application.ID, target.Name).Error; err == nil {
		target.ID = existingTarget.ID
		target.CreatedAt = existingTarget.CreatedAt
		target.CreatedBy = firstNonEmpty(existingTarget.CreatedBy, user.ID)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return systemComponentApplicationPlan{}, false
	}
	target.EnvironmentID = target.ID

	secretValue := model.SecretValue{
		ID:        id.New("sec"),
		CipherRef: secret.Encrypt(reportToken),
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
		EnvironmentID:      target.ID,
		DeploymentTargetID: target.ID,
		ImageRef:           target.ImageRef,
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
		Namespace:          "ns-" + shortID(project.ID),
		Status:             "deploying",
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

func (h *Handlers) persistSystemComponentApplicationPlan(plan systemComponentApplicationPlan) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&plan.Application).Error; err != nil {
			return err
		}
		if err := tx.Save(&plan.DeploymentTarget).Error; err != nil {
			return err
		}
		revision, err := nextReleaseRevisionFor(tx, plan.Release.ProjectID, plan.Release.ApplicationID, plan.Release.DeploymentTargetID)
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

func systemComponentProbeEnv(cluster model.RuntimeCluster, componentID string, mode string, apiBaseURL string) string {
	values := map[string]string{
		"API_BASE_URL":        strings.TrimRight(apiBaseURL, "/"),
		"COMPONENT_ID":        componentID,
		"RUNTIME_CLUSTER_ID":  cluster.ID,
		"CONTROLLER_TYPE":     firstNonEmpty(cluster.GatewayControllerType, "traefik"),
		"MODE":                firstNonEmpty(mode, "traefik-metrics"),
		"GATEWAY_NAMESPACE":   firstNonEmpty(cluster.GatewayNamespace, "kube-system"),
		"TRAEFIK_METRICS_URL": "http://traefik." + firstNonEmpty(cluster.GatewayNamespace, "kube-system") + ".svc.cluster.local:9100/metrics",
		"PROBE_ADDR":          ":9090",
		"SCRAPE_INTERVAL":     "60s",
	}
	content, err := json.Marshal(values)
	if err != nil {
		return "{}"
	}
	return string(content)
}

func (h *Handlers) systemComponentForBearerToken(token string, componentID string) (model.SystemComponentInstallation, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return model.SystemComponentInstallation{}, false
	}
	var installation model.SystemComponentInstallation
	err := h.db.First(&installation, "component_id = ? and report_token_hash = ?", componentID, hashToken(token)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.SystemComponentInstallation{}, false
	}
	return installation, err == nil
}
