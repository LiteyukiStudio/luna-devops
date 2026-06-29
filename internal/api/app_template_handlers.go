package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type appTemplateInstallInput struct {
	ApplicationName string            `json:"applicationName"`
	ApplicationSlug string            `json:"applicationSlug"`
	DeploymentName  string            `json:"deploymentName"`
	Stage           string            `json:"stage"`
	ClusterID       string            `json:"clusterId"`
	Namespace       string            `json:"namespace"`
	ImageRef        string            `json:"imageRef"`
	Replicas        int               `json:"replicas"`
	CPURequest      string            `json:"cpuRequest"`
	MemoryRequest   string            `json:"memoryRequest"`
	DataCapacity    string            `json:"dataCapacity"`
	InstallNow      *bool             `json:"installNow"`
	Values          map[string]string `json:"values"`
}

type appTemplateInstallResponse struct {
	Installation     model.AppTemplateInstallation `json:"installation"`
	Application      model.Application             `json:"application"`
	DeploymentTarget deploymentTargetResponse      `json:"deploymentTarget"`
	Release          *model.Release                `json:"release,omitempty"`
}

func (h *Handlers) ListAppTemplates(ctx *gin.Context) {
	templates, err := appstore.Catalog()
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, templates)
}

func (h *Handlers) InstallAppTemplate(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	project, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) || !h.ensureBillingAllowsDeployChange(ctx, project.ID) {
		return
	}

	template, found, err := appstore.Find(ctx.Param("templateId"))
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeError(ctx, http.StatusNotFound, "应用模板不存在")
		return
	}

	var input appTemplateInstallInput
	if !bindJSON(ctx, &input) {
		return
	}
	plan, ok := h.buildTemplateInstallPlan(ctx, user, project, template, input)
	if !ok {
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&plan.Application).Error; err != nil {
			return err
		}
		if err := tx.Create(&plan.DeploymentTarget).Error; err != nil {
			return err
		}
		if plan.Release != nil {
			revision, err := nextReleaseRevisionFor(tx, plan.Release.ProjectID, plan.Release.ApplicationID, plan.Release.DeploymentTargetID)
			if err != nil {
				return err
			}
			plan.Release.Revision = revision
			if err := tx.Create(plan.Release).Error; err != nil {
				return err
			}
			plan.Installation.ReleaseID = plan.Release.ID
			plan.Installation.Status = "deploying"
		}
		if err := tx.Create(&plan.Installation).Error; err != nil {
			return err
		}
		for _, entry := range plan.SecretValues {
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}

	for _, entry := range plan.SecretValues {
		h.audit(user.ID, "secret.write", entry.ID, true, entry.Resource)
	}
	h.audit(user.ID, "app_template.install", plan.Installation.ID, true, template.ID)

	if plan.Release != nil && !h.enqueueDeployRun(ctx.Request.Context(), *plan.Release) {
		message := "部署任务投递失败"
		_ = h.db.Model(&model.Release{}).Where("id = ?", plan.Release.ID).Updates(map[string]any{"status": "failed", "message": message}).Error
		_ = h.db.Model(&model.AppTemplateInstallation{}).Where("id = ?", plan.Installation.ID).Updates(map[string]any{"status": "deploy_failed", "message": message}).Error
		plan.Release.Status = "failed"
		plan.Release.Message = message
		plan.Installation.Status = "deploy_failed"
		plan.Installation.Message = message
		h.audit(user.ID, "app_template.deploy_enqueue", plan.Installation.ID, false, message)
	}

	ctx.JSON(http.StatusCreated, appTemplateInstallResponse{
		Installation:     plan.Installation,
		Application:      plan.Application,
		DeploymentTarget: deploymentTargetResponseFromModel(plan.DeploymentTarget),
		Release:          plan.Release,
	})
}

type templateInstallPlan struct {
	Application      model.Application
	DeploymentTarget model.DeploymentTarget
	Installation     model.AppTemplateInstallation
	Release          *model.Release
	SecretValues     []model.SecretValue
}

func (h *Handlers) buildTemplateInstallPlan(ctx *gin.Context, user model.User, project model.Project, template appstore.Template, input appTemplateInstallInput) (templateInstallPlan, bool) {
	applicationID := id.New("app")
	targetID := id.New("dplt")
	installationID := id.New("atpl")
	applicationSlug := strings.TrimSpace(input.ApplicationSlug)
	if applicationSlug == "" {
		applicationSlug = fallbackTemplateSlug(template.Slug, applicationID)
	}
	if len(applicationSlug) > applicationSlugMaxLength {
		writeError(ctx, http.StatusBadRequest, fmt.Sprintf("应用标识最多 %d 个字符", applicationSlugMaxLength))
		return templateInstallPlan{}, false
	}
	if !h.ensureApplicationSlugAvailable(ctx, project.ID, applicationSlug, "") {
		return templateInstallPlan{}, false
	}

	rendered, err := appstore.Render(template, input.Values)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return templateInstallPlan{}, false
	}
	secretRefs, secretEntries, ok := templateSecretRefs(ctx, user.ID, installationID, rendered.SecretEnv)
	if !ok {
		return templateInstallPlan{}, false
	}
	secretRefsContent, err := json.Marshal(secretRefs)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return templateInstallPlan{}, false
	}
	envContent, err := json.Marshal(rendered.Env)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return templateInstallPlan{}, false
	}
	valuesSnapshot, err := json.Marshal(safeTemplateValues(template, rendered.Values))
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return templateInstallPlan{}, false
	}

	replicas := input.Replicas
	if replicas <= 0 {
		replicas = template.DefaultReplicas
	}
	if replicas <= 0 {
		replicas = 1
	}
	cpuRequest, ok := normalizeBuildResourceQuantity(ctx, firstNonEmpty(input.CPURequest, template.DefaultCPU), "1", "运行 CPU")
	if !ok {
		return templateInstallPlan{}, false
	}
	memoryRequest, ok := normalizeBuildResourceQuantity(ctx, firstNonEmpty(input.MemoryRequest, template.DefaultMemory), "1Gi", "运行内存")
	if !ok {
		return templateInstallPlan{}, false
	}
	dataCapacity, ok := normalizeDataCapacity(ctx, firstNonEmpty(input.DataCapacity, template.DataCapacity), template.DataRetentionEnabled)
	if !ok {
		return templateInstallPlan{}, false
	}
	dataMountPath, ok := normalizeDataMountPath(ctx, template.DataMountPath, template.DataRetentionEnabled)
	if !ok {
		return templateInstallPlan{}, false
	}
	dataVolumes, ok := normalizeDataVolumes(ctx, "", template.DataRetentionEnabled, dataMountPath, dataCapacity)
	if !ok {
		return templateInstallPlan{}, false
	}
	dataVolumesContent, err := json.Marshal(dataVolumes)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return templateInstallPlan{}, false
	}
	configFilesContent, ok := templateConfigFiles(ctx, rendered.ConfigFiles)
	if !ok {
		return templateInstallPlan{}, false
	}
	secretFilesContent, secretFileEntries, ok := h.templateSecretFiles(ctx, user.ID, installationID, rendered.SecretFiles)
	if !ok {
		return templateInstallPlan{}, false
	}
	secretEntries = append(secretEntries, secretFileEntries...)
	clusterID := strings.TrimSpace(input.ClusterID)
	if clusterID == "" {
		clusterID = h.defaultRuntimeClusterID()
	}
	if clusterID != "" && !h.runtimeClusterExists(ctx, clusterID) {
		return templateInstallPlan{}, false
	}

	installNow := true
	if input.InstallNow != nil {
		installNow = *input.InstallNow
	}
	applicationName := strings.TrimSpace(input.ApplicationName)
	if applicationName == "" {
		applicationName = template.Name
	}
	deploymentName := strings.TrimSpace(input.DeploymentName)
	if deploymentName == "" {
		deploymentName = "default"
	}
	imageRef := strings.TrimSpace(input.ImageRef)
	if imageRef == "" {
		imageRef = strings.TrimSpace(template.Image)
	}
	if imageRef == "" {
		writeError(ctx, http.StatusBadRequest, "镜像地址不能为空")
		return templateInstallPlan{}, false
	}

	application := model.Application{
		ID:                applicationID,
		ProjectID:         project.ID,
		Slug:              applicationSlug,
		Name:              applicationName,
		Icon:              templateApplicationIcon(template),
		DeleteStatus:      "active",
		DataRetentionMode: "retain",
	}
	target := model.DeploymentTarget{
		ID:                   targetID,
		ProjectID:            project.ID,
		ApplicationID:        applicationID,
		EnvironmentID:        targetID,
		Name:                 deploymentName,
		Stage:                normalizeStage(input.Stage),
		ClusterID:            clusterID,
		Namespace:            strings.TrimSpace(input.Namespace),
		Replicas:             replicas,
		CPURequest:           cpuRequest,
		MemoryRequest:        memoryRequest,
		ServicePort:          fallbackInt(template.ServicePort, 8080),
		ServicePorts:         model.EncodeDeploymentServicePorts([]model.DeploymentServicePort{{Name: "http", Port: fallbackInt(template.ServicePort, 8080)}}, fallbackInt(template.ServicePort, 8080)),
		SourceType:           "image",
		ImageRef:             imageRef,
		BuildCPURequest:      defaultBuildCPURequest,
		BuildMemoryRequest:   defaultBuildMemoryRequest,
		BuildTimeoutSeconds:  defaultBuildTimeoutSeconds,
		ConcurrencyPolicy:    "queue",
		EnvVars:              string(envContent),
		SecretRefs:           string(secretRefsContent),
		ConfigFiles:          configFilesContent,
		SecretFiles:          secretFilesContent,
		DataRetentionEnabled: template.DataRetentionEnabled,
		DataCapacity:         dataCapacity,
		DataMountPath:        dataMountPath,
		DataVolumes:          string(dataVolumesContent),
		Enabled:              true,
		DeleteStatus:         "active",
		CreatedBy:            user.ID,
	}
	installation := model.AppTemplateInstallation{
		ID:                 installationID,
		TemplateID:         template.ID,
		TemplateVersion:    template.Version,
		ProjectID:          project.ID,
		ApplicationID:      applicationID,
		DeploymentTargetID: targetID,
		Status:             "installed",
		ValuesSnapshot:     string(valuesSnapshot),
		CreatedBy:          user.ID,
	}
	var release *model.Release
	if installNow {
		release = &model.Release{
			ID:                 id.New("rel"),
			ProjectID:          project.ID,
			ApplicationID:      applicationID,
			EnvironmentID:      targetID,
			DeploymentTargetID: targetID,
			ImageRef:           target.ImageRef,
			Type:               "deploy",
			Status:             "pending",
			Message:            "app template install",
			CreatedBy:          user.ID,
		}
	}
	return templateInstallPlan{
		Application:      application,
		DeploymentTarget: target,
		Installation:     installation,
		Release:          release,
		SecretValues:     secretEntries,
	}, true
}

func templateSecretRefs(ctx *gin.Context, userID string, installationID string, values map[string]string) (map[string]string, []model.SecretValue, bool) {
	output := map[string]string{}
	entries := []model.SecretValue{}
	for key, value := range values {
		if !isBuildEnvKey(key) {
			writeError(ctx, http.StatusBadRequest, "密钥变量名只能使用字母、数字和下划线，且不能以数字开头")
			return nil, nil, false
		}
		cipherRef := secret.Encrypt(value)
		if cipherRef == "" {
			writeError(ctx, http.StatusInternalServerError, "密钥加密失败")
			return nil, nil, false
		}
		entry := model.SecretValue{
			ID:        id.New("sec"),
			CipherRef: cipherRef,
			CreatedBy: userID,
			Resource:  "app_template:" + installationID + ":secret:" + key,
		}
		output[key] = "secret-id:" + entry.ID
		entries = append(entries, entry)
	}
	return output, entries, true
}

func templateConfigFiles(ctx *gin.Context, files []appstore.ConfigFile) (string, bool) {
	if len(files) == 0 {
		return "", true
	}
	content, err := json.Marshal(files)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return "", false
	}
	return normalizeRuntimeConfigFilesInput(ctx, string(content))
}

func (h *Handlers) templateSecretFiles(ctx *gin.Context, userID string, installationID string, files []appstore.ConfigFile) (string, []model.SecretValue, bool) {
	if len(files) == 0 {
		return "", nil, true
	}
	refs := map[string]string{}
	entries := []model.SecretValue{}
	for _, file := range files {
		filePath, ok := normalizeRuntimeConfigFilePathInput(ctx, file.Path)
		if !ok {
			return "", nil, false
		}
		if _, exists := refs[filePath]; exists {
			writeError(ctx, http.StatusBadRequest, "密钥文件路径不能重复")
			return "", nil, false
		}
		content := strings.TrimSpace(file.Content)
		if content == "" {
			continue
		}
		cipherRef := secret.Encrypt(content)
		if cipherRef == "" {
			writeError(ctx, http.StatusInternalServerError, "密钥加密失败")
			return "", nil, false
		}
		entry := model.SecretValue{
			ID:        id.New("sec"),
			CipherRef: cipherRef,
			CreatedBy: userID,
			Resource:  "app_template:" + installationID + ":file:" + filePath,
		}
		refs[filePath] = "secret-id:" + entry.ID
		entries = append(entries, entry)
	}
	content, err := json.Marshal(refs)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return "", nil, false
	}
	return string(content), entries, true
}

func (h *Handlers) defaultRuntimeClusterID() string {
	var cluster model.RuntimeCluster
	err := h.db.Where("type in ? and is_default = ?", []string{"kubernetes", "k3s"}, true).Order("created_at asc").First(&cluster).Error
	if err == nil {
		return cluster.ID
	}
	err = h.db.Where("type in ?", []string{"kubernetes", "k3s"}).Order("created_at asc").First(&cluster).Error
	if err == nil {
		return cluster.ID
	}
	return ""
}

func (h *Handlers) runtimeClusterExists(ctx *gin.Context, clusterID string) bool {
	var count int64
	if err := h.db.Model(&model.RuntimeCluster{}).Where("id = ? and type in ?", clusterID, []string{"kubernetes", "k3s"}).Count(&count).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}
	if count == 0 {
		writeError(ctx, http.StatusBadRequest, "运行集群不存在")
		return false
	}
	return true
}

func safeTemplateValues(template appstore.Template, values map[string]string) map[string]string {
	secretKeys := map[string]bool{}
	for _, definition := range template.Values {
		if definition.Secret {
			secretKeys[definition.Key] = true
		}
	}
	output := map[string]string{}
	for key, value := range values {
		if secretKeys[key] {
			output[key] = "set"
			continue
		}
		output[key] = value
	}
	return output
}

func fallbackTemplateSlug(slug string, appID string) string {
	base := strings.TrimSpace(slug)
	if base == "" {
		base = "app"
	}
	suffix := shortID(appID)
	value := base + "-" + suffix
	if len(value) <= applicationSlugMaxLength {
		return value
	}
	maxBase := applicationSlugMaxLength - len(suffix) - 1
	if maxBase < 1 {
		return suffix
	}
	if len(base) > maxBase {
		base = base[:maxBase]
	}
	return strings.Trim(base, "-") + "-" + suffix
}

func templateApplicationIcon(template appstore.Template) string {
	if icon := strings.TrimSpace(template.Icon); isApplicationIconReference(icon) {
		return icon
	}
	switch strings.TrimSpace(template.Category) {
	case "database":
		return "database"
	case "middleware":
		return "server"
	default:
		return "box"
	}
}

func shortID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return id.New("x")
	}
	if index := strings.LastIndex(value, "_"); index >= 0 && index+1 < len(value) {
		value = value[index+1:]
	}
	if len(value) > 8 {
		return value[:8]
	}
	return value
}
