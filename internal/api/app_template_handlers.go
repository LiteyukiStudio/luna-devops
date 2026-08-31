package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/resourceidentifier"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type appTemplateInstallInput struct {
	ApplicationName       string            `json:"applicationName"`
	ApplicationIdentifier string            `json:"applicationIdentifier"`
	DeploymentName        string            `json:"deploymentName"`
	Stage                 string            `json:"stage"`
	ClusterID             string            `json:"clusterId"`
	Namespace             string            `json:"namespace"`
	ImageRef              string            `json:"imageRef"`
	Replicas              int               `json:"replicas"`
	CPURequest            string            `json:"cpuRequest"`
	MemoryRequest         string            `json:"memoryRequest"`
	ProjectVolumeID       string            `json:"projectVolumeId"`
	InstallNow            *bool             `json:"installNow"`
	Values                map[string]string `json:"values"`
}

type appTemplateInstallResponse struct {
	Installation     model.AppTemplateInstallation `json:"installation"`
	Application      model.Application             `json:"application"`
	DeploymentTarget deploymentTargetResponse      `json:"deploymentTarget"`
	Release          *model.Release                `json:"release,omitempty"`
}

type appTemplateSummaryResponse struct {
	ID                 string                `json:"id"`
	Slug               string                `json:"slug"`
	Name               string                `json:"name"`
	Description        string                `json:"description"`
	Category           string                `json:"category"`
	Kind               string                `json:"kind"`
	SystemComponent    string                `json:"systemComponent"`
	Icon               string                `json:"icon"`
	OfficialWebsite    string                `json:"officialWebsite"`
	OfficialRepository string                `json:"officialRepository"`
	PopularityWeight   int                   `json:"popularityWeight"`
	Image              string                `json:"image"`
	Version            string                `json:"version"`
	ServicePort        int                   `json:"servicePort"`
	DefaultReplicas    int                   `json:"defaultReplicas"`
	DefaultCPU         string                `json:"defaultCPU"`
	DefaultMemory      string                `json:"defaultMemory"`
	DataVolumes        []appstore.DataVolume `json:"dataVolumes"`
	ValueCount         int                   `json:"valueCount"`
	RequiredValueCount int                   `json:"requiredValueCount"`
}

type appTemplateValueResponse struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	Description  string `json:"description"`
	Default      string `json:"default"`
	Required     bool   `json:"required"`
	Secret       bool   `json:"secret"`
	AutoGenerate bool   `json:"autoGenerate"`
}

type appTemplateResponse struct {
	appTemplateSummaryResponse
	Values []appTemplateValueResponse `json:"values"`
}

func (h *Handlers) ListAppTemplates(ctx *gin.Context) {
	templates, err := appstore.Catalog()
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	query := strings.ToLower(strings.TrimSpace(ctx.Query("query")))
	category := strings.ToLower(strings.TrimSpace(ctx.Query("category")))
	items := make([]appTemplateSummaryResponse, 0, len(templates))
	for _, template := range templates {
		if category != "" && strings.ToLower(template.Category) != category {
			continue
		}
		searchable := strings.ToLower(strings.Join([]string{
			template.ID, template.Slug, template.Name, template.Description, template.Category,
			template.Image, template.OfficialWebsite, template.OfficialRepository,
		}, " "))
		if query != "" && !strings.Contains(searchable, query) {
			continue
		}
		items = append(items, appTemplateSummaryFrom(template))
	}
	ctx.JSON(http.StatusOK, items)
}

func (h *Handlers) GetAppTemplate(ctx *gin.Context) {
	template, found, err := appstore.Find(ctx.Param("templateId"))
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeError(ctx, http.StatusNotFound, "应用模板不存在")
		return
	}
	ctx.JSON(http.StatusOK, appTemplateDetailFrom(template))
}

func appTemplateSummaryFrom(template appstore.Template) appTemplateSummaryResponse {
	requiredValueCount := 0
	for _, definition := range template.Values {
		if definition.Required {
			requiredValueCount++
		}
	}
	return appTemplateSummaryResponse{
		ID: template.ID, Slug: template.Slug, Name: template.Name, Description: template.Description,
		Category: template.Category, Kind: template.Kind, SystemComponent: template.SystemComponent,
		Icon: template.Icon, OfficialWebsite: template.OfficialWebsite, OfficialRepository: template.OfficialRepository,
		PopularityWeight: template.PopularityWeight, Image: template.Image, Version: template.Version,
		ServicePort: template.ServicePort, DefaultReplicas: template.DefaultReplicas,
		DefaultCPU: template.DefaultCPU, DefaultMemory: template.DefaultMemory, DataVolumes: template.DataVolumes,
		ValueCount: len(template.Values), RequiredValueCount: requiredValueCount,
	}
}

func appTemplateDetailFrom(template appstore.Template) appTemplateResponse {
	values := make([]appTemplateValueResponse, 0, len(template.Values))
	for _, definition := range template.Values {
		defaultValue := definition.Default
		if definition.Secret {
			defaultValue = ""
		}
		values = append(values, appTemplateValueResponse{
			Key: definition.Key, Label: definition.Label, Description: definition.Description,
			Default: defaultValue, Required: definition.Required, Secret: definition.Secret,
			AutoGenerate: definition.AutoGenerate,
		})
	}
	return appTemplateResponse{appTemplateSummaryResponse: appTemplateSummaryFrom(template), Values: values}
}

func (h *Handlers) InstallAppTemplate(ctx *gin.Context) {
	user, project, ok := h.authorizeProject(ctx, authz.ActionProjectWrite)
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

	mountChanges := deploymentVolumeMountChanges{}
	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		if err := createApplicationRecord(tx, &plan.Application); err != nil {
			return err
		}
		if err := tx.Create(&plan.DeploymentTarget).Error; err != nil {
			return err
		}
		var syncErr error
		mountChanges, syncErr = syncDeploymentTargetVolumeMounts(ctx.Request.Context(), tx, plan.DeploymentTarget, plan.DataVolumes)
		if syncErr != nil {
			return syncErr
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
	}); errors.Is(err, errApplicationIdentifierExists) {
		writeApplicationIdentifierConflict(ctx, "active")
		return
	} else if err != nil {
		h.auditDeploymentVolumeMountFailure(ctx.Request.Context(), user.ID, mountChanges, err)
		if volume.ErrorCode(err) != "" {
			writeVolumeError(ctx, err)
		} else {
			writeError(ctx, http.StatusBadRequest, err.Error())
		}
		return
	}

	for _, entry := range plan.SecretValues {
		h.auditWithContext(user.ID, "secret.write", entry.ID, true, entry.Resource, ctx.Request.Context())
	}
	h.auditDeploymentVolumeMountChanges(ctx.Request.Context(), user.ID, plan.DeploymentTarget, mountChanges)
	h.auditWithContext(user.ID, "app_template.install", plan.Installation.ID, true, template.ID, ctx.Request.Context())

	if plan.Release != nil && !h.enqueueDeployRun(ctx.Request.Context(), *plan.Release) {
		message := "部署任务投递失败"
		_ = h.dbFor(ctx).Model(&model.Release{}).Where("id = ?", plan.Release.ID).Updates(map[string]any{"status": "failed", "message": message}).Error
		_ = h.dbFor(ctx).Model(&model.AppTemplateInstallation{}).Where("id = ?", plan.Installation.ID).Updates(map[string]any{"status": "deploy_failed", "message": message}).Error
		plan.Release.Status = "failed"
		plan.Release.Message = message
		plan.Installation.Status = "deploy_failed"
		plan.Installation.Message = message
		h.auditWithContext(user.ID, "app_template.deploy_enqueue", plan.Installation.ID, false, message, ctx.Request.Context())
	}

	mountsByTarget, err := h.deploymentTargetVolumeMountsByTarget(ctx.Request.Context(), []model.DeploymentTarget{plan.DeploymentTarget})
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, appTemplateInstallResponse{
		Installation:     plan.Installation,
		Application:      plan.Application,
		DeploymentTarget: deploymentTargetResponseFromModel(plan.DeploymentTarget, mountsByTarget[plan.DeploymentTarget.ID]),
		Release:          plan.Release,
	})
}

type templateInstallPlan struct {
	Application      model.Application
	DeploymentTarget model.DeploymentTarget
	DataVolumes      []deploymentTargetDataVolumeInput
	Installation     model.AppTemplateInstallation
	Release          *model.Release
	SecretValues     []model.SecretValue
}

func (h *Handlers) buildTemplateInstallPlan(ctx *gin.Context, user model.User, project model.Project, template appstore.Template, input appTemplateInstallInput) (templateInstallPlan, bool) {
	installationID := id.New("atpl")
	applicationIdentifier := strings.TrimSpace(input.ApplicationIdentifier)
	if applicationIdentifier == "" {
		applicationIdentifier = fallbackTemplateIdentifier(template.Slug, installationID)
	}
	if err := resourceidentifier.Validate(applicationIdentifier, applicationIdentifierMinLength, applicationIdentifierMaxLength); err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "application.identifier_invalid", err.Error())
		return templateInstallPlan{}, false
	}
	if !h.ensureApplicationIdentifierAvailable(ctx, project.ID, applicationIdentifier, "") {
		return templateInstallPlan{}, false
	}
	applicationID := id.New("app")
	stage, validStage := normalizePublicStage(input.Stage)
	if !validStage {
		writeDeploymentStageInvalid(ctx, "stage", "deployment stage must be dev, test, staging, or prod")
		return templateInstallPlan{}, false
	}
	if err := resourceidentifier.Validate(stage, stageIdentifierMinLength, stageIdentifierMaxLength); err != nil {
		writeDeploymentStageInvalid(ctx, "stage", err.Error())
		return templateInstallPlan{}, false
	}
	targetID := id.New("dplt")

	rendered, err := appstore.Render(template, input.Values)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return templateInstallPlan{}, false
	}
	secretRefs, secretEntries, ok := h.templateSecretRefs(ctx, user.ID, installationID, rendered.SecretEnv)
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
	clusterID := strings.TrimSpace(input.ClusterID)
	if clusterID == "" {
		clusterID = h.defaultRuntimeClusterID(ctx.Request.Context())
	}
	if _, ok := h.runtimeClusterForProjectUse(ctx, user, project.ID, clusterID); !ok {
		return templateInstallPlan{}, false
	}
	namespace := runtimeProjectNamespace(project)
	if requestedNamespace := strings.TrimSpace(input.Namespace); requestedNamespace != "" {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment_target.namespace_forbidden", "deployment target namespace is managed by the project and cannot be overridden")
		return templateInstallPlan{}, false
	}
	projectVolumeID := strings.TrimSpace(input.ProjectVolumeID)
	projectVolumeCount := 0
	var projectVolumeDeclaration *appstore.DataVolume
	for index := range template.DataVolumes {
		if template.DataVolumes[index].SourceType == "projectVolume" {
			projectVolumeCount++
			projectVolumeDeclaration = &template.DataVolumes[index]
		}
	}
	if projectVolumeCount > 1 {
		writeErrorCode(ctx, http.StatusInternalServerError, "app_template.volume_contract_invalid", "应用模板包含多个项目数据卷声明")
		return templateInstallPlan{}, false
	}
	var selectedProjectVolume *model.ProjectVolume
	if projectVolumeCount == 1 {
		if projectVolumeID == "" {
			writeErrorCode(ctx, http.StatusBadRequest, "app_template.project_volume_required", "该模板需要选择一个可挂载的项目数据卷")
			return templateInstallPlan{}, false
		}
		var projectVolume model.ProjectVolume
		if err := h.dbFor(ctx).First(&projectVolume, "id = ? and project_id = ?", projectVolumeID, project.ID).Error; err != nil {
			writeErrorCode(ctx, http.StatusBadRequest, "project_volume.not_found", "项目数据卷不存在")
			return templateInstallPlan{}, false
		}
		if !volume.CanAttachProjectVolume(projectVolume) {
			writeErrorCode(ctx, http.StatusConflict, "project_volume.not_attachable", "项目数据卷当前不可挂载")
			return templateInstallPlan{}, false
		}
		if projectVolume.ClusterID != clusterID {
			writeErrorCode(ctx, http.StatusConflict, "project_volume.cluster_mismatch", "项目数据卷与部署目标必须位于同一集群")
			return templateInstallPlan{}, false
		}
		if projectVolume.Namespace != namespace {
			writeErrorCode(ctx, http.StatusConflict, "project_volume.namespace_mismatch", "项目数据卷与部署目标必须位于同一命名空间")
			return templateInstallPlan{}, false
		}
		if projectVolume.VolumeMode != model.ProjectVolumeModeFilesystem {
			if projectVolumeDeclaration == nil || projectVolumeDeclaration.DevicePath == "" {
				writeErrorCode(ctx, http.StatusConflict, "project_volume.mode_incompatible", "项目数据卷模式与应用模板不兼容")
				return templateInstallPlan{}, false
			}
		}
		if projectVolume.VolumeMode == model.ProjectVolumeModeFilesystem && (projectVolumeDeclaration == nil || projectVolumeDeclaration.MountPath == "") {
			writeErrorCode(ctx, http.StatusConflict, "project_volume.mode_incompatible", "项目数据卷模式与应用模板不兼容")
			return templateInstallPlan{}, false
		}
		selectedProjectVolume = &projectVolume
	} else if projectVolumeID != "" {
		writeErrorCode(ctx, http.StatusBadRequest, "app_template.project_volume_unsupported", "该模板不使用项目数据卷")
		return templateInstallPlan{}, false
	}
	dataVolumes := appTemplateDeploymentDataVolumes(template, selectedProjectVolume)
	dataVolumes, ok = normalizeDataVolumes(ctx, dataVolumes)
	if !ok {
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
		ID:           applicationID,
		ProjectID:    project.ID,
		Identifier:   applicationIdentifier,
		Name:         applicationName,
		Icon:         templateApplicationIcon(template),
		DeleteStatus: "active",
	}
	target := model.DeploymentTarget{
		ID:                  targetID,
		ProjectID:           project.ID,
		ApplicationID:       applicationID,
		EnvironmentID:       targetID,
		Name:                deploymentName,
		Stage:               stage,
		KubernetesName:      resourceidentifier.DeploymentTargetName(applicationIdentifier, stage),
		ClusterID:           clusterID,
		Replicas:            replicas,
		CPURequest:          cpuRequest,
		MemoryRequest:       memoryRequest,
		ContainerCommand:    strings.TrimSpace(template.ContainerCommand),
		ContainerArgs:       strings.TrimSpace(template.ContainerArgs),
		ServicePorts:        model.EncodeDeploymentServicePorts([]model.DeploymentServicePort{{Name: "http", Port: fallbackInt(template.ServicePort, 8080)}}, fallbackInt(template.ServicePort, 8080)),
		SourceType:          "image",
		ImageRef:            imageRef,
		BuildCPURequest:     defaultBuildCPURequest,
		BuildMemoryRequest:  defaultBuildMemoryRequest,
		BuildTimeoutSeconds: defaultBuildTimeoutSeconds,
		ConcurrencyPolicy:   "queue",
		EnvVars:             string(envContent),
		SecretRefs:          string(secretRefsContent),
		ConfigFiles:         configFilesContent,
		SecretFiles:         secretFilesContent,
		Enabled:             true,
		DeleteStatus:        "active",
		CreatedBy:           user.ID,
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
		DataVolumes:      dataVolumes,
		Installation:     installation,
		Release:          release,
		SecretValues:     secretEntries,
	}, true
}

func appTemplateDeploymentDataVolumes(template appstore.Template, selectedProjectVolume *model.ProjectVolume) []deploymentTargetDataVolumeInput {
	dataVolumes := make([]deploymentTargetDataVolumeInput, 0, len(template.DataVolumes))
	for _, declaration := range template.DataVolumes {
		input := deploymentTargetDataVolumeInput{
			LogicalName: declaration.LogicalName, SourceType: declaration.SourceType,
			MountPath: declaration.MountPath, DevicePath: declaration.DevicePath, ReadOnly: declaration.ReadOnly,
		}
		if declaration.SourceType == "projectVolume" && selectedProjectVolume != nil {
			input.ProjectVolumeID = selectedProjectVolume.ID
		}
		if declaration.SourceType == "emptyDir" && declaration.EmptyDir != nil {
			input.EmptyDir = &deploymentTargetEmptyDirInput{Medium: declaration.EmptyDir.Medium, SizeLimit: declaration.EmptyDir.SizeLimit}
		}
		dataVolumes = append(dataVolumes, input)
	}
	return dataVolumes
}

func (h *Handlers) templateSecretRefs(ctx *gin.Context, userID string, installationID string, values map[string]string) (map[string]string, []model.SecretValue, bool) {
	output := map[string]string{}
	entries := []model.SecretValue{}
	for key, value := range values {
		if !isBuildEnvKey(key) {
			writeError(ctx, http.StatusBadRequest, "密钥变量名只能使用字母、数字和下划线，且不能以数字开头")
			return nil, nil, false
		}
		cipherRef := h.secrets.Encrypt(value)
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
		cipherRef := h.secrets.Encrypt(content)
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

func (h *Handlers) defaultRuntimeClusterID(ctx context.Context) string {
	var cluster model.RuntimeCluster
	query := runtimecluster.ActiveScope(h.dbWithContext(ctx))
	err := query.Where("type in ? and is_default = ?", []string{"kubernetes", "k3s"}, true).Order("created_at asc").First(&cluster).Error
	if err == nil {
		return cluster.ID
	}
	err = query.Where("type in ?", []string{"kubernetes", "k3s"}).Order("created_at asc").First(&cluster).Error
	if err == nil {
		return cluster.ID
	}
	return ""
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
			if strings.TrimSpace(value) != "" {
				output[key] = "set"
			}
			continue
		}
		output[key] = value
	}
	return output
}

func fallbackTemplateIdentifier(slug string, appID string) string {
	base := strings.TrimSpace(slug)
	if base == "" {
		base = "app"
	}
	suffix := shortID(appID)
	value := base + "-" + suffix
	if len(value) <= applicationIdentifierMaxLength {
		return value
	}
	maxBase := applicationIdentifierMaxLength - len(suffix) - 1
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
