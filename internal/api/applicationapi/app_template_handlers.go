package applicationapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/dependency"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/resourceidentifier"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type appTemplateInstallInput struct {
	ApplicationName           string            `json:"applicationName"`
	ApplicationIdentifier     string            `json:"applicationIdentifier"`
	DeploymentName            string            `json:"deploymentName"`
	Stage                     string            `json:"stage"`
	ClusterID                 string            `json:"clusterId"`
	Namespace                 string            `json:"namespace"`
	ImageRef                  string            `json:"imageRef"`
	Replicas                  int               `json:"replicas"`
	CPURequest                string            `json:"cpuRequest"`
	MemoryRequest             string            `json:"memoryRequest"`
	ProjectVolumeID           string            `json:"projectVolumeId"`
	ComponentProjectVolumeIDs map[string]string `json:"componentProjectVolumeIds"`
	InstallNow                *bool             `json:"installNow"`
	Values                    map[string]string `json:"values"`
}

type appTemplateInstallResponse struct {
	Installation     model.AppTemplateInstallation           `json:"installation"`
	Application      model.Application                       `json:"application"`
	DeploymentTarget any                                     `json:"deploymentTarget"`
	Release          *model.Release                          `json:"release,omitempty"`
	Components       []appTemplateInstalledComponentResponse `json:"components,omitempty"`
}

type appTemplateInstalledComponentResponse struct {
	Key              string            `json:"key"`
	Application      model.Application `json:"application"`
	DeploymentTarget any               `json:"deploymentTarget"`
	Release          *model.Release    `json:"release,omitempty"`
}

type appTemplateComponentResponse struct {
	Key             string                `json:"key"`
	Name            string                `json:"name"`
	Image           string                `json:"image"`
	ServicePort     int                   `json:"servicePort"`
	DefaultReplicas int                   `json:"defaultReplicas"`
	DefaultCPU      string                `json:"defaultCPU"`
	DefaultMemory   string                `json:"defaultMemory"`
	DataVolumes     []appstore.DataVolume `json:"dataVolumes"`
}

type appTemplateSummaryResponse struct {
	ID                 string                         `json:"id"`
	Slug               string                         `json:"slug"`
	Name               string                         `json:"name"`
	Description        string                         `json:"description"`
	Category           string                         `json:"category"`
	Kind               string                         `json:"kind"`
	SystemComponent    string                         `json:"systemComponent"`
	Icon               string                         `json:"icon"`
	OfficialWebsite    string                         `json:"officialWebsite"`
	OfficialRepository string                         `json:"officialRepository"`
	PopularityWeight   int                            `json:"popularityWeight"`
	Featured           bool                           `json:"featured"`
	Image              string                         `json:"image"`
	Version            string                         `json:"version"`
	ServicePort        int                            `json:"servicePort"`
	DefaultReplicas    int                            `json:"defaultReplicas"`
	DefaultCPU         string                         `json:"defaultCPU"`
	DefaultMemory      string                         `json:"defaultMemory"`
	DataVolumes        []appstore.DataVolume          `json:"dataVolumes"`
	Components         []appTemplateComponentResponse `json:"components"`
	ValueCount         int                            `json:"valueCount"`
	RequiredValueCount int                            `json:"requiredValueCount"`
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
		writeTemplateCatalogUnavailable(ctx, err)
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
		writeTemplateCatalogUnavailable(ctx, err)
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
	components := make([]appTemplateComponentResponse, 0, len(template.Components))
	for _, component := range template.Components {
		components = append(components, appTemplateComponentResponse{
			Key: component.Key, Name: component.Name, Image: component.Image, ServicePort: component.ServicePort,
			DefaultReplicas: component.DefaultReplicas, DefaultCPU: component.DefaultCPU,
			DefaultMemory: component.DefaultMemory, DataVolumes: nonNilDataVolumes(component.DataVolumes),
		})
	}
	return appTemplateSummaryResponse{
		ID: template.ID, Slug: template.Slug, Name: template.Name, Description: template.Description,
		Category: template.Category, Kind: template.Kind, SystemComponent: template.SystemComponent,
		Icon: template.Icon, OfficialWebsite: template.OfficialWebsite, OfficialRepository: template.OfficialRepository,
		PopularityWeight: template.PopularityWeight, Featured: template.Featured, Image: template.Image, Version: template.Version,
		ServicePort: template.ServicePort, DefaultReplicas: template.DefaultReplicas,
		DefaultCPU: template.DefaultCPU, DefaultMemory: template.DefaultMemory, DataVolumes: nonNilDataVolumes(template.DataVolumes),
		Components: components, ValueCount: len(template.Values), RequiredValueCount: requiredValueCount,
	}
}

func nonNilDataVolumes(volumes []appstore.DataVolume) []appstore.DataVolume {
	if volumes == nil {
		return []appstore.DataVolume{}
	}
	return volumes
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
		writeTemplateCatalogUnavailable(ctx, err)
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

	mountChanges := make([]deploymentVolumeMountChanges, len(plan.Components))
	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		for index := range plan.Components {
			component := &plan.Components[index]
			if err := createApplicationRecord(tx, &component.Application); err != nil {
				return err
			}
			if err := tx.Create(&component.DeploymentTarget).Error; err != nil {
				return err
			}
			var syncErr error
			mountChanges[index], syncErr = h.host.SyncDeploymentTargetVolumeMounts(ctx.Request.Context(), tx, component.DeploymentTarget, component.DataVolumes)
			if syncErr != nil {
				return syncErr
			}
			if component.Release != nil {
				revision, err := h.host.NextReleaseRevisionFor(tx, component.Release.ProjectID, component.Release.ApplicationID, component.Release.DeploymentTargetID)
				if err != nil {
					return err
				}
				component.Release.Revision = revision
				if err := tx.Create(component.Release).Error; err != nil {
					return err
				}
			}
		}
		if entry := plan.Components[0]; entry.Release != nil {
			plan.Installation.ReleaseID = entry.Release.ID
		}
		if err := tx.Create(&plan.Installation).Error; err != nil {
			return err
		}
		if template.IsMultiComponent() {
			for _, component := range plan.Components {
				record := model.AppTemplateInstallationComponent{
					ID: id.New("atpc"), InstallationID: plan.Installation.ID, ComponentKey: component.Key,
					ProjectID: project.ID, ApplicationID: component.Application.ID, DeploymentTargetID: component.DeploymentTarget.ID,
				}
				if component.Release != nil {
					record.ReleaseID = component.Release.ID
				}
				if err := tx.Create(&record).Error; err != nil {
					return err
				}
			}
		}
		for index := range plan.ServiceBindings {
			if err := tx.Create(&plan.ServiceBindings[index]).Error; err != nil {
				return err
			}
		}
		for _, entry := range plan.SecretValues {
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		for _, changes := range mountChanges {
			h.auditDeploymentVolumeMountFailure(ctx.Request.Context(), user.ID, changes, err)
		}
		h.auditWithContext(user.ID, "app_template.install", plan.Installation.ID, false, appTemplateInstallErrorCode(err), ctx.Request.Context())
		h.writeTemplateInstallError(ctx, err)
		return
	}

	for _, entry := range plan.SecretValues {
		h.auditWithContext(user.ID, "secret.write", entry.ID, true, entry.Resource, ctx.Request.Context())
	}
	for _, binding := range plan.ServiceBindings {
		h.auditWithContext(user.ID, "service_binding.create", binding.ID, true, binding.SourceDeploymentTargetID, ctx.Request.Context())
	}
	for index, component := range plan.Components {
		h.auditDeploymentVolumeMountChanges(ctx.Request.Context(), user.ID, component.DeploymentTarget, mountChanges[index])
	}
	h.auditWithContext(user.ID, "app_template.install", plan.Installation.ID, true, template.ID, ctx.Request.Context())

	for _, component := range plan.Components {
		if component.Release == nil || h.enqueueDeployRun(ctx.Request.Context(), *component.Release) {
			continue
		}
		if err := markTemplateReleaseDispatchFailed(h.dbFor(ctx), component.Release); err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		h.auditWithContext(user.ID, "app_template.deploy_enqueue", plan.Installation.ID, false, model.ReleaseFailureDispatchFailed, ctx.Request.Context())
	}

	targets := make([]model.DeploymentTarget, 0, len(plan.Components))
	for _, component := range plan.Components {
		targets = append(targets, component.DeploymentTarget)
	}
	mountsByTarget, err := h.deploymentTargetVolumeMountsByTarget(ctx.Request.Context(), targets)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	entry := plan.Components[0]
	response := appTemplateInstallResponse{
		Installation:     plan.Installation,
		Application:      entry.Application,
		DeploymentTarget: h.host.DeploymentTargetResponseFromModel(entry.DeploymentTarget, mountsByTarget[entry.DeploymentTarget.ID]),
		Release:          entry.Release,
	}
	if template.IsMultiComponent() {
		for _, component := range plan.Components {
			response.Components = append(response.Components, appTemplateInstalledComponentResponse{
				Key:              component.Key,
				Application:      component.Application,
				DeploymentTarget: h.host.DeploymentTargetResponseFromModel(component.DeploymentTarget, mountsByTarget[component.DeploymentTarget.ID]),
				Release:          component.Release,
			})
		}
	}
	ctx.JSON(http.StatusCreated, response)
}

// markTemplateReleaseDispatchFailed 终止投递失败的模板安装发布。状态守卫保证 Worker 已推进或其他写入者已收敛的发布不被覆盖；
// 响应中的发布以数据库回读为准。
func markTemplateReleaseDispatchFailed(db *gorm.DB, release *model.Release) error {
	finishedAt := time.Now()
	if err := db.Model(&model.Release{}).
		Where("id = ? and status in ?", release.ID, model.ReleaseActiveStatuses).
		Updates(map[string]any{
			"status":       "failed",
			"failure_code": model.ReleaseFailureDispatchFailed,
			"message":      "deploy task dispatch failed",
			"finished_at":  &finishedAt,
		}).Error; err != nil {
		return err
	}
	return db.First(release, "id = ?", release.ID).Error
}

type templateInstallPlan struct {
	Installation model.AppTemplateInstallation
	// Components 按模板声明顺序排列，第一个是入口组件；单组件模板只有一个 Key 为空的组件。
	Components []templateComponentPlan
	// ServiceBindings 把组件间 {{components.<key>.host}} 引用落成服务关系，进入拓扑并阻止被依赖组件单独删除。
	ServiceBindings []model.ServiceBinding
	SecretValues    []model.SecretValue
}

type templateComponentPlan struct {
	Key              string
	Application      model.Application
	DeploymentTarget model.DeploymentTarget
	DataVolumes      []deploymentTargetDataVolumeInput
	Release          *model.Release
}

// templateInstallComponents 把单组件模板视为一个 Key 为空的组件，使安装只有一条组件化路径。
func templateInstallComponents(template appstore.Template) []appstore.Component {
	if template.IsMultiComponent() {
		return template.Components
	}
	return []appstore.Component{{Name: template.Name, Workload: template.Workload}}
}

func (h *Handlers) buildTemplateInstallPlan(ctx *gin.Context, user model.User, project model.Project, template appstore.Template, input appTemplateInstallInput) (templateInstallPlan, bool) {
	components := templateInstallComponents(template)
	if template.IsMultiComponent() {
		if strings.TrimSpace(input.ImageRef) != "" || input.Replicas > 0 || strings.TrimSpace(input.CPURequest) != "" || strings.TrimSpace(input.MemoryRequest) != "" || strings.TrimSpace(input.ProjectVolumeID) != "" {
			writeErrorCode(ctx, http.StatusBadRequest, "app_template.component_override_unsupported", "多组件模板按组件定义运行，不接受单镜像、副本、规格或 projectVolumeId 覆盖")
			return templateInstallPlan{}, false
		}
	} else if len(input.ComponentProjectVolumeIDs) > 0 {
		writeErrorCode(ctx, http.StatusBadRequest, "app_template.project_volume_unsupported", "单组件模板使用 projectVolumeId 选择项目数据卷")
		return templateInstallPlan{}, false
	}

	installationID := id.New("atpl")
	longestSuffix := 0
	for _, component := range components {
		if component.Key != "" {
			longestSuffix = max(longestSuffix, len(component.Key)+1)
		}
	}
	baseIdentifier := strings.TrimSpace(input.ApplicationIdentifier)
	if baseIdentifier == "" {
		baseIdentifier = fallbackTemplateIdentifier(template.Slug, installationID, applicationIdentifierMaxLength-longestSuffix)
	}
	stage, validStage := h.host.NormalizePublicStage(input.Stage)
	if !validStage {
		h.host.WriteDeploymentStageInvalid(ctx, "stage", "deployment stage must be dev, test, staging, or prod")
		return templateInstallPlan{}, false
	}
	if err := resourceidentifier.Validate(stage, stageIdentifierMinLength, stageIdentifierMaxLength); err != nil {
		h.host.WriteDeploymentStageInvalid(ctx, "stage", err.Error())
		return templateInstallPlan{}, false
	}
	identifiers := make([]string, len(components))
	for index, component := range components {
		identifiers[index] = baseIdentifier
		if component.Key != "" {
			identifiers[index] = baseIdentifier + "-" + component.Key
		}
		if err := resourceidentifier.Validate(identifiers[index], applicationIdentifierMinLength, applicationIdentifierMaxLength); err != nil {
			writeErrorCode(ctx, http.StatusBadRequest, "application.identifier_invalid", err.Error())
			return templateInstallPlan{}, false
		}
		if !h.ensureApplicationIdentifierAvailable(ctx, project.ID, identifiers[index], "") {
			return templateInstallPlan{}, false
		}
	}

	values, err := appstore.ResolveValues(template, input.Values)
	if err != nil {
		writeTemplateValuesError(ctx, err)
		return templateInstallPlan{}, false
	}
	valuesSnapshot, err := json.Marshal(safeTemplateValues(template, values))
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return templateInstallPlan{}, false
	}
	kubernetesNames := make([]string, len(components))
	for index, component := range components {
		kubernetesNames[index] = resourceidentifier.DeploymentTargetName(identifiers[index], stage)
		if component.Key != "" {
			values[appstore.ComponentHostValueKey(component.Key)] = kubernetesNames[index]
		}
	}

	clusterID := strings.TrimSpace(input.ClusterID)
	if clusterID == "" {
		clusterID = h.defaultRuntimeClusterID(ctx.Request.Context())
	}
	if _, ok := h.runtimeClusterForProjectUse(ctx, user, project.ID, clusterID); !ok {
		return templateInstallPlan{}, false
	}
	namespace := h.host.RuntimeProjectNamespace(project)
	if requestedNamespace := strings.TrimSpace(input.Namespace); requestedNamespace != "" {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment_target.namespace_forbidden", "deployment target namespace is managed by the project and cannot be overridden")
		return templateInstallPlan{}, false
	}
	componentKeys := map[string]bool{}
	for _, component := range components {
		componentKeys[component.Key] = true
	}
	for key := range input.ComponentProjectVolumeIDs {
		if !componentKeys[key] {
			writeErrorCode(ctx, http.StatusBadRequest, "app_template.project_volume_unsupported", "componentProjectVolumeIds 包含模板中不存在的组件")
			return templateInstallPlan{}, false
		}
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

	plan := templateInstallPlan{Components: make([]templateComponentPlan, 0, len(components))}
	selectedVolumeIDs := map[string]bool{}
	for index, component := range components {
		secretResource := "app_template:" + installationID
		if component.Key != "" {
			secretResource += ":" + component.Key
		}
		rendered := appstore.RenderWorkload(component.Workload, values)
		secretRefs, secretEntries, ok := h.templateSecretRefs(ctx, user.ID, secretResource, rendered.SecretEnv)
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
		replicas := input.Replicas
		if replicas <= 0 {
			replicas = component.DefaultReplicas
		}
		if replicas <= 0 {
			replicas = 1
		}
		cpuRequest, ok := h.host.NormalizeBuildResourceQuantity(ctx, firstNonEmpty(input.CPURequest, component.DefaultCPU), "1", "运行 CPU")
		if !ok {
			return templateInstallPlan{}, false
		}
		memoryRequest, ok := h.host.NormalizeBuildResourceQuantity(ctx, firstNonEmpty(input.MemoryRequest, component.DefaultMemory), "1Gi", "运行内存")
		if !ok {
			return templateInstallPlan{}, false
		}
		projectVolumeID := strings.TrimSpace(firstNonEmpty(input.ProjectVolumeID, input.ComponentProjectVolumeIDs[component.Key]))
		if projectVolumeID != "" && selectedVolumeIDs[projectVolumeID] {
			writeErrorCode(ctx, http.StatusBadRequest, "app_template.project_volume_duplicated", "同一个项目数据卷不能同时挂载给多个组件")
			return templateInstallPlan{}, false
		}
		selectedVolumeIDs[projectVolumeID] = projectVolumeID != ""
		selectedProjectVolume, ok := h.templateProjectVolume(ctx, project.ID, clusterID, namespace, component.Workload, projectVolumeID)
		if !ok {
			return templateInstallPlan{}, false
		}
		dataVolumes, ok := h.host.NormalizeDataVolumes(ctx, appTemplateDeploymentDataVolumes(component.Workload, selectedProjectVolume))
		if !ok {
			return templateInstallPlan{}, false
		}
		configFilesContent, ok := h.templateConfigFiles(ctx, rendered.ConfigFiles)
		if !ok {
			return templateInstallPlan{}, false
		}
		secretFilesContent, secretFileEntries, ok := h.templateSecretFiles(ctx, user.ID, secretResource, rendered.SecretFiles)
		if !ok {
			return templateInstallPlan{}, false
		}
		plan.SecretValues = append(append(plan.SecretValues, secretEntries...), secretFileEntries...)
		imageRef := strings.TrimSpace(firstNonEmpty(input.ImageRef, component.Image))
		if imageRef == "" {
			writeError(ctx, http.StatusBadRequest, "镜像地址不能为空")
			return templateInstallPlan{}, false
		}
		name := applicationName
		if component.Key != "" {
			name = applicationName + " · " + component.Name
		}
		applicationID := id.New("app")
		targetID := id.New("dplt")
		servicePort := fallbackInt(component.ServicePort, 8080)
		componentPlan := templateComponentPlan{
			Key: component.Key,
			Application: model.Application{
				ID: applicationID, ProjectID: project.ID, Identifier: identifiers[index], Name: name,
				Icon: templateApplicationIcon(template), DeleteStatus: "active",
			},
			DeploymentTarget: model.DeploymentTarget{
				ID:                  targetID,
				ProjectID:           project.ID,
				ApplicationID:       applicationID,
				Name:                deploymentName,
				Stage:               stage,
				KubernetesName:      kubernetesNames[index],
				ClusterID:           clusterID,
				Replicas:            replicas,
				CPURequest:          cpuRequest,
				MemoryRequest:       memoryRequest,
				ContainerCommand:    strings.TrimSpace(component.ContainerCommand),
				ContainerArgs:       strings.TrimSpace(component.ContainerArgs),
				FSGroup:             strings.TrimSpace(component.FSGroup),
				ServicePorts:        model.EncodeDeploymentServicePorts([]model.DeploymentServicePort{{Name: "http", Port: servicePort}}, servicePort),
				SourceType:          "image",
				ImageRef:            imageRef,
				BuildCPURequest:     defaultBuildCPURequest,
				BuildMemoryRequest:  defaultBuildMemoryRequest,
				BuildTimeoutSeconds: defaultBuildTimeoutSeconds,
				EnvVars:             string(envContent),
				SecretRefs:          string(secretRefsContent),
				ConfigFiles:         configFilesContent,
				SecretFiles:         secretFilesContent,
				Enabled:             true,
				DeleteStatus:        "active",
				CreatedBy:           user.ID,
			},
			DataVolumes: dataVolumes,
		}
		if installNow {
			componentPlan.Release = &model.Release{
				ID: id.New("rel"), ProjectID: project.ID, ApplicationID: applicationID, DeploymentTargetID: targetID,
				ImageRef: imageRef, Type: "deploy", Status: "pending", Message: "app template install", CreatedBy: user.ID,
			}
		}
		plan.Components = append(plan.Components, componentPlan)
	}
	entry := plan.Components[0]
	plan.Installation = model.AppTemplateInstallation{
		ID:                 installationID,
		TemplateID:         template.ID,
		TemplateVersion:    template.Version,
		ProjectID:          project.ID,
		ApplicationID:      entry.Application.ID,
		DeploymentTargetID: entry.DeploymentTarget.ID,
		ValuesSnapshot:     string(valuesSnapshot),
		CreatedBy:          user.ID,
	}
	plan.ServiceBindings, err = templateComponentServiceBindings(components, plan.Components, project.ID, user.ID)
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "app_template.dependency_invalid", err.Error())
		return templateInstallPlan{}, false
	}
	return plan, true
}

// templateComponentServiceBindings 为每个组件对其他组件的引用创建一条 host/port 服务关系。
// 渲染出的连接串仍使用同命名空间短主机名；服务关系额外注入 <KEY>_HOST/<KEY>_PORT，并作为依赖事实源。
func templateComponentServiceBindings(components []appstore.Component, plans []templateComponentPlan, projectID, actorID string) ([]model.ServiceBinding, error) {
	planByKey := make(map[string]templateComponentPlan, len(plans))
	for _, componentPlan := range plans {
		planByKey[componentPlan.Key] = componentPlan
	}
	bindings := []model.ServiceBinding{}
	for index, component := range components {
		source := plans[index]
		for _, dependencyKey := range appstore.ComponentDependencies(component.Workload) {
			target, exists := planByKey[dependencyKey]
			if !exists {
				return nil, fmt.Errorf("component %s references unknown component %s", component.Key, dependencyKey)
			}
			hostEnv, portEnv := appstore.ComponentBindingEnvVars(dependencyKey)
			binding := model.ServiceBinding{ID: id.New("sbind"), ProjectID: projectID, CreatedBy: actorID}
			if err := dependency.PrepareServiceBinding(&binding, dependency.ServiceBindingInput{
				SourceApplicationID: source.Application.ID, SourceDeploymentTargetID: source.DeploymentTarget.ID,
				TargetApplicationID: target.Application.ID, TargetDeploymentTargetID: target.DeploymentTarget.ID,
				TargetPortName: "http", Protocol: "tcp", InjectionMode: "host_port", HostEnvVar: hostEnv, PortEnvVar: portEnv,
			}, source.DeploymentTarget, target.DeploymentTarget); err != nil {
				return nil, fmt.Errorf("component %s binding to %s: %w", component.Key, dependencyKey, err)
			}
			bindings = append(bindings, binding)
		}
	}
	return bindings, nil
}

func writeTemplateCatalogUnavailable(ctx *gin.Context, err error) {
	writeErrorCode(ctx, http.StatusInternalServerError, "app_template.catalog_unavailable", err.Error())
}

func writeTemplateValuesError(ctx *gin.Context, err error) {
	var required *appstore.ValueRequiredError
	switch {
	case errors.As(err, &required):
		writeErrorCode(ctx, http.StatusBadRequest, "app_template.value_required", err.Error())
	case errors.Is(err, appstore.ErrRandomSecretUnavailable):
		writeErrorCode(ctx, http.StatusInternalServerError, "app_template.secret_generation_failed", err.Error())
	default:
		writeErrorCode(ctx, http.StatusInternalServerError, "app_template.install_failed", err.Error())
	}
}

// appTemplateInstallErrorCode 把安装事务错误映射为稳定错误码；未知数据库或运行时错误统一为 install_failed，不回显原文。
func appTemplateInstallErrorCode(err error) string {
	switch {
	case errors.Is(err, errApplicationIdentifierExists):
		return "application.identifier_exists"
	case volume.ErrorCode(err) != "":
		return volume.ErrorCode(err)
	case strings.Contains(err.Error(), "SQLSTATE 23505"):
		return "app_template.install_conflict"
	default:
		return "app_template.install_failed"
	}
}

func (h *Handlers) writeTemplateInstallError(ctx *gin.Context, err error) {
	switch code := appTemplateInstallErrorCode(err); code {
	case "application.identifier_exists":
		writeApplicationIdentifierConflict(ctx, "active")
	case "app_template.install_conflict":
		writeErrorCode(ctx, http.StatusConflict, code, err.Error())
	case "app_template.install_failed":
		writeErrorCode(ctx, http.StatusInternalServerError, code, err.Error())
	default:
		h.host.WriteVolumeError(ctx, err)
	}
}

// templateProjectVolume 校验组件声明与用户选择的项目数据卷是否匹配；组件不使用项目数据卷时返回 nil。
func (h *Handlers) templateProjectVolume(ctx *gin.Context, projectID, clusterID, namespace string, workload appstore.Workload, projectVolumeID string) (*model.ProjectVolume, bool) {
	var declaration *appstore.DataVolume
	for index := range workload.DataVolumes {
		if workload.DataVolumes[index].SourceType == "projectVolume" {
			declaration = &workload.DataVolumes[index]
		}
	}
	if declaration == nil {
		if projectVolumeID != "" {
			writeErrorCode(ctx, http.StatusBadRequest, "app_template.project_volume_unsupported", "该模板不使用项目数据卷")
			return nil, false
		}
		return nil, true
	}
	if projectVolumeID == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "app_template.project_volume_required", "该模板需要选择一个可挂载的项目数据卷")
		return nil, false
	}
	var projectVolume model.ProjectVolume
	if err := h.dbFor(ctx).First(&projectVolume, "id = ? and project_id = ?", projectVolumeID, projectID).Error; err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "project_volume.not_found", "项目数据卷不存在")
		return nil, false
	}
	if !volume.CanAttachProjectVolume(projectVolume) {
		writeErrorCode(ctx, http.StatusConflict, "project_volume.not_attachable", "项目数据卷当前不可挂载")
		return nil, false
	}
	if projectVolume.ClusterID != clusterID {
		writeErrorCode(ctx, http.StatusConflict, "project_volume.cluster_mismatch", "项目数据卷与部署目标必须位于同一集群")
		return nil, false
	}
	if projectVolume.Namespace != namespace {
		writeErrorCode(ctx, http.StatusConflict, "project_volume.namespace_mismatch", "项目数据卷与部署目标必须位于同一命名空间")
		return nil, false
	}
	compatible := declaration.DevicePath != ""
	if projectVolume.VolumeMode == model.ProjectVolumeModeFilesystem {
		compatible = declaration.MountPath != ""
	}
	if !compatible {
		writeErrorCode(ctx, http.StatusConflict, "project_volume.mode_incompatible", "项目数据卷模式与应用模板不兼容")
		return nil, false
	}
	return &projectVolume, true
}

func appTemplateDeploymentDataVolumes(template appstore.Workload, selectedProjectVolume *model.ProjectVolume) []deploymentTargetDataVolumeInput {
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

func (h *Handlers) templateSecretRefs(ctx *gin.Context, userID string, secretResource string, values map[string]string) (map[string]string, []model.SecretValue, bool) {
	output := map[string]string{}
	entries := []model.SecretValue{}
	for key, value := range values {
		if !h.host.IsBuildEnvKey(key) {
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
			Resource:  secretResource + ":secret:" + key,
		}
		output[key] = "secret-id:" + entry.ID
		entries = append(entries, entry)
	}
	return output, entries, true
}

func (h *Handlers) templateConfigFiles(ctx *gin.Context, files []appstore.ConfigFile) (string, bool) {
	if len(files) == 0 {
		return "", true
	}
	content, err := json.Marshal(files)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return "", false
	}
	return h.host.NormalizeRuntimeConfigFilesInput(ctx, string(content))
}

func (h *Handlers) templateSecretFiles(ctx *gin.Context, userID string, secretResource string, files []appstore.ConfigFile) (string, []model.SecretValue, bool) {
	if len(files) == 0 {
		return "", nil, true
	}
	refs := map[string]string{}
	entries := []model.SecretValue{}
	for _, file := range files {
		filePath, ok := h.host.NormalizeRuntimeConfigFilePathInput(ctx, file.Path)
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
			Resource:  secretResource + ":file:" + filePath,
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
	cluster, err := runtimecluster.ResolveForTarget(h.dbWithContext(ctx), "")
	if err != nil {
		return ""
	}
	return cluster.ID
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

func fallbackTemplateIdentifier(slug string, appID string, maxLength int) string {
	base := strings.TrimSpace(slug)
	if base == "" {
		base = "app"
	}
	suffix := shortID(appID)
	value := base + "-" + suffix
	if len(value) <= maxLength {
		return value
	}
	maxBase := maxLength - len(suffix) - 1
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
