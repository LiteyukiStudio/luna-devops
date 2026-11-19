package applicationapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

func isSystemComponentTemplate(tpl appstore.Template) bool {
	return strings.TrimSpace(tpl.Kind) == "system_component" || strings.TrimSpace(tpl.SystemComponent) != ""
}

// --- Tenant Template AI Tools (3) ---

type deliveryListTemplatesInput struct {
	Query     *string `json:"query,omitempty" desc:"按模板名称、标识或简介搜索"`
	Category  *string `json:"category,omitempty" enum:"collaboration|database|developerTool|middleware|observability|security|storage" desc:"模板分类过滤"`
	SortBy    *string `json:"sortBy,omitempty" enum:"popularityWeight|name|createdAt" default:"popularityWeight" desc:"排序字段"`
	SortOrder *string `json:"sortOrder,omitempty" enum:"asc|desc" default:"desc" desc:"排序方向"`
	Page      *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码"`
	PageSize  *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（上限 50）"`
}

type deliveryGetTemplateInput struct {
	Template string `json:"template" desc:"租户应用模板 ID 或 slug"`
}

type deliveryInstallTemplateInput struct {
	Project               string            `json:"project" desc:"项目 ID 或 identifier"`
	Template              string            `json:"template" desc:"租户应用模板 ID 或 slug"`
	Stage                 string            `json:"stage" enum:"dev|test|staging|prod" desc:"目标部署阶段"`
	ApplicationName       *string           `json:"applicationName,omitempty" desc:"应用显示名称（省略默认使用模板名称）"`
	ApplicationIdentifier *string           `json:"applicationIdentifier,omitempty" desc:"应用唯一英文标识（省略自动生成；多组件下作为组件标识前缀）"`
	Cluster               *string           `json:"cluster,omitempty" desc:"目标运行集群 ID 或名称（省略使用项目默认集群）"`
	Values                map[string]string `json:"values,omitempty" desc:"普通模板参数（仅允许 secret=false 参数键）"`
	SecretValues          map[string]string `json:"secretValues,omitempty" sensitive:"true" desc:"敏感模板参数（仅允许 secret=true 参数键；省略时对 autoGenerate=true 项自动生成）"`
	ProjectVolume         *string           `json:"projectVolume,omitempty" desc:"单组件模板需挂载持久化卷时传入 ProjectVolume ID 或 displayName（与 componentVolumes 互斥）"`
	ComponentVolumes      map[string]string `json:"componentVolumes,omitempty" desc:"多组件模板按 components[].key 映射各组件独立 ProjectVolume ID 或 displayName"`
	InstallNow            *bool             `json:"installNow,omitempty" default:"true" desc:"是否立即创建并投递首个 Release"`
}

type installTemplateLockedPayload struct {
	ProjectID                 string            `json:"projectId"`
	TemplateID                string            `json:"templateId"`
	ClusterID                 string            `json:"clusterId"`
	BaseApplicationIdentifier string            `json:"baseApplicationIdentifier"`
	ProjectVolumeID           string            `json:"projectVolumeId,omitempty"`
	ComponentVolumeIDs        map[string]string `json:"componentVolumeIds,omitempty"`
}

type aiInstalledComponentSummary struct {
	Key                   string `json:"key"`
	ApplicationID         string `json:"applicationId"`
	ApplicationIdentifier string `json:"applicationIdentifier"`
	ApplicationName       string `json:"applicationName"`
	DeploymentTargetID    string `json:"deploymentTargetId"`
	ReleaseID             string `json:"releaseId,omitempty"`
}

func (h *Handler) resolveRuntimeClusterByRef(tc *aitool.ToolContext, user model.User, projectID, clusterRef string) (model.RuntimeCluster, error) {
	clusterRef = strings.TrimSpace(clusterRef)
	if clusterRef == "" {
		clusterID := h.defaultRuntimeClusterID(tc.Context())
		c, rec := newAIToolGin(tc)
		cl, ok := h.runtimeClusterForProjectUse(c, user, projectID, clusterID)
		if !ok {
			return model.RuntimeCluster{}, aiToolErrorFromRecorder(rec, "runtime_cluster.not_available", "no default runtime cluster is available for this project")
		}
		return cl, nil
	}

	var byID model.RuntimeCluster
	if err := tc.DB.First(&byID, "id = ? AND delete_status <> ?", clusterRef, "deleted").Error; err == nil {
		c, rec := newAIToolGin(tc)
		cl, ok := h.runtimeClusterForProjectUse(c, user, projectID, byID.ID)
		if !ok {
			return model.RuntimeCluster{}, aiToolErrorFromRecorder(rec, "runtime_cluster.access_denied", "runtime cluster is not available for this project")
		}
		return cl, nil
	}

	var candidates []model.RuntimeCluster
	if err := tc.DB.Where("LOWER(name) = LOWER(?) AND delete_status <> ?", clusterRef, "deleted").
		Order("created_at asc").
		Find(&candidates).Error; err != nil {
		return model.RuntimeCluster{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	// Filter to clusters usable by (user, projectID) so private clusters of other tenants neither leak nor cause false ambiguity.
	usable := make([]model.RuntimeCluster, 0, len(candidates))
	for _, cand := range candidates {
		c, _ := newAIToolGin(tc)
		if cl, ok := h.runtimeClusterForProjectUse(c, user, projectID, cand.ID); ok {
			usable = append(usable, cl)
		}
	}
	resolved, err := aitool.ResolveUniqueByIDOrName(
		usable,
		"runtime_cluster",
		"runtime_cluster.not_found",
		clusterRef,
		func(item model.RuntimeCluster) string { return item.ID },
		func(item model.RuntimeCluster) string { return item.Name },
	)
	if err != nil {
		return model.RuntimeCluster{}, err
	}
	return resolved, nil
}

func resolveProjectVolumeByRef(db *gorm.DB, projectID, ref string) (model.ProjectVolume, error) {
	var volumes []model.ProjectVolume
	if err := db.Where("project_id = ?", projectID).Order("created_at asc").Find(&volumes).Error; err != nil {
		return model.ProjectVolume{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return aitool.ResolveUniqueByIDOrName(
		volumes,
		"project_volume",
		"project_volume.not_found",
		ref,
		func(v model.ProjectVolume) string { return v.ID },
		func(v model.ProjectVolume) string { return v.DisplayName },
	)
}

func validateAndMergeTemplateValues(tpl appstore.Template, plainValues, secretValues map[string]string) (map[string]string, []string, error) {
	defs := make(map[string]appstore.ValueDefinition, len(tpl.Values))
	allowedPlain := make([]string, 0, len(tpl.Values))
	allowedSecret := make([]string, 0, len(tpl.Values))
	for _, d := range tpl.Values {
		key := strings.TrimSpace(d.Key)
		defs[key] = d
		if d.Secret || d.AutoGenerate {
			allowedSecret = append(allowedSecret, key)
		}
		if !d.Secret {
			allowedPlain = append(allowedPlain, key)
		}
	}
	sort.Strings(allowedPlain)
	sort.Strings(allowedSecret)

	merged := make(map[string]string, len(plainValues)+len(secretValues))
	sensitiveKeys := make([]string, 0, len(secretValues)+4)

	for rawKey, val := range plainValues {
		key := strings.TrimSpace(rawKey)
		def, exists := defs[key]
		if !exists {
			return nil, nil, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"argument_invalid",
				fmt.Sprintf("unknown template value key %q", key),
				fmt.Sprintf("Call delivery_get_template(%q) to inspect declared parameter keys (values: [%s], secretValues: [%s]).", tpl.Slug, strings.Join(allowedPlain, ", "), strings.Join(allowedSecret, ", ")),
			)
		}
		if def.Secret {
			return nil, nil, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"argument_invalid",
				fmt.Sprintf("template parameter %q is sensitive (secret=true) and cannot be passed in values", key),
				fmt.Sprintf("Move %q from values to secretValues so it is encrypted and redacted end-to-end.", key),
			)
		}
		merged[key] = val
	}

	for rawKey, val := range secretValues {
		key := strings.TrimSpace(rawKey)
		def, exists := defs[key]
		if !exists {
			return nil, nil, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"argument_invalid",
				fmt.Sprintf("unknown template secret key %q", key),
				fmt.Sprintf("Call delivery_get_template(%q) to inspect declared secretValues keys ([%s]).", tpl.Slug, strings.Join(allowedSecret, ", ")),
			)
		}
		if !def.Secret && !def.AutoGenerate {
			return nil, nil, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"argument_invalid",
				fmt.Sprintf("template parameter %q is not a secret (secret=false) and cannot be passed in secretValues", key),
				fmt.Sprintf("Move %q from secretValues to values.", key),
			)
		}
		merged[key] = val
		if strings.TrimSpace(val) != "" {
			sensitiveKeys = append(sensitiveKeys, key)
		}
	}

	for _, def := range tpl.Values {
		if (def.Secret || def.AutoGenerate) && strings.TrimSpace(merged[def.Key]) == "" && def.AutoGenerate {
			sensitiveKeys = append(sensitiveKeys, def.Key)
		}
	}
	slices.Sort(sensitiveKeys)
	sensitiveKeys = slices.Compact(sensitiveKeys)
	return merged, sensitiveKeys, nil
}

func (h *Handler) prepareAITemplateInstall(
	tc *aitool.ToolContext,
	in deliveryInstallTemplateInput,
	locked *installTemplateLockedPayload,
) (model.User, model.Project, appstore.Template, templateInstallPlan, installTemplateLockedPayload, []string, error) {
	user, project, _, _, err := h.authorizeAIProject(tc, in.Project, authz.ActionProjectWrite)
	if err != nil {
		return model.User{}, model.Project{}, appstore.Template{}, templateInstallPlan{}, installTemplateLockedPayload{}, nil, err
	}
	if err := h.ensureAIProjectMutable(tc, project); err != nil {
		return model.User{}, model.Project{}, appstore.Template{}, templateInstallPlan{}, installTemplateLockedPayload{}, nil, err
	}
	if err := h.ensureAIBillingAllowsDeploy(tc, project.ID); err != nil {
		return model.User{}, model.Project{}, appstore.Template{}, templateInstallPlan{}, installTemplateLockedPayload{}, nil, err
	}

	tpl, found, err := appstore.FindWithPlatform(tc.Context(), tc.DB, in.Template)
	if err != nil {
		return model.User{}, model.Project{}, appstore.Template{}, templateInstallPlan{}, installTemplateLockedPayload{}, nil,
			aitool.NewError(http.StatusInternalServerError, "app_template.catalog_unavailable", err.Error())
	}
	if !found || isSystemComponentTemplate(tpl) {
		return model.User{}, model.Project{}, appstore.Template{}, templateInstallPlan{}, installTemplateLockedPayload{}, nil,
			aitool.NotFound("app_template.not_found", "application template was not found")
	}
	if locked != nil && (locked.ProjectID != project.ID || locked.TemplateID != tpl.ID) {
		return model.User{}, model.Project{}, appstore.Template{}, templateInstallPlan{}, installTemplateLockedPayload{}, nil,
			aitool.Conflict("ai.preview_lock_mismatch", "preview lock does not match target project or template")
	}

	if in.ProjectVolume != nil && strings.TrimSpace(*in.ProjectVolume) != "" && len(in.ComponentVolumes) > 0 {
		return model.User{}, model.Project{}, appstore.Template{}, templateInstallPlan{}, installTemplateLockedPayload{}, nil,
			aitool.BadRequest("argument_invalid", "projectVolume and componentVolumes are mutually exclusive")
	}

	mergedValues, sensitiveKeys, err := validateAndMergeTemplateValues(tpl, in.Values, in.SecretValues)
	if err != nil {
		return model.User{}, model.Project{}, appstore.Template{}, templateInstallPlan{}, installTemplateLockedPayload{}, nil, err
	}

	var clusterID string
	if locked != nil && strings.TrimSpace(locked.ClusterID) != "" {
		c, rec := newAIToolGin(tc)
		cl, ok := h.runtimeClusterForProjectUse(c, user, project.ID, locked.ClusterID)
		if !ok {
			return model.User{}, model.Project{}, appstore.Template{}, templateInstallPlan{}, installTemplateLockedPayload{}, nil,
				aiToolErrorFromRecorder(rec, "runtime_cluster.access_denied", "locked runtime cluster is no longer usable")
		}
		clusterID = cl.ID
	} else {
		clusterRef := ""
		if in.Cluster != nil {
			clusterRef = *in.Cluster
		}
		cl, err := h.resolveRuntimeClusterByRef(tc, user, project.ID, clusterRef)
		if err != nil {
			return model.User{}, model.Project{}, appstore.Template{}, templateInstallPlan{}, installTemplateLockedPayload{}, nil, err
		}
		clusterID = cl.ID
	}

	var resolvedProjectVolumeID string
	resolvedComponentVolumeIDs := map[string]string{}
	if locked != nil {
		resolvedProjectVolumeID = locked.ProjectVolumeID
		if len(locked.ComponentVolumeIDs) > 0 {
			resolvedComponentVolumeIDs = maps.Clone(locked.ComponentVolumeIDs)
		}
	} else {
		if in.ProjectVolume != nil && strings.TrimSpace(*in.ProjectVolume) != "" {
			if tpl.IsMultiComponent() {
				return model.User{}, model.Project{}, appstore.Template{}, templateInstallPlan{}, installTemplateLockedPayload{}, nil,
					aitool.BadRequest("app_template.component_override_unsupported", "multi-component templates require componentVolumes instead of projectVolume")
			}
			vol, err := resolveProjectVolumeByRef(tc.DB, project.ID, *in.ProjectVolume)
			if err != nil {
				return model.User{}, model.Project{}, appstore.Template{}, templateInstallPlan{}, installTemplateLockedPayload{}, nil, err
			}
			resolvedProjectVolumeID = vol.ID
		}
		if len(in.ComponentVolumes) > 0 {
			if !tpl.IsMultiComponent() {
				return model.User{}, model.Project{}, appstore.Template{}, templateInstallPlan{}, installTemplateLockedPayload{}, nil,
					aitool.BadRequest("app_template.project_volume_unsupported", "single-component templates use projectVolume instead of componentVolumes")
			}
			for compKey, volRef := range in.ComponentVolumes {
				if strings.TrimSpace(volRef) == "" {
					continue
				}
				vol, err := resolveProjectVolumeByRef(tc.DB, project.ID, volRef)
				if err != nil {
					return model.User{}, model.Project{}, appstore.Template{}, templateInstallPlan{}, installTemplateLockedPayload{}, nil, err
				}
				resolvedComponentVolumeIDs[strings.TrimSpace(compKey)] = vol.ID
			}
		}
	}

	components := templateInstallComponents(tpl)
	longestSuffix := 0
	for _, comp := range components {
		if comp.Key != "" {
			longestSuffix = max(longestSuffix, len(comp.Key)+1)
		}
	}
	baseIdentifier := ""
	if locked != nil && strings.TrimSpace(locked.BaseApplicationIdentifier) != "" {
		baseIdentifier = locked.BaseApplicationIdentifier
	} else if in.ApplicationIdentifier != nil && strings.TrimSpace(*in.ApplicationIdentifier) != "" {
		baseIdentifier = strings.TrimSpace(*in.ApplicationIdentifier)
	} else {
		baseIdentifier = fallbackTemplateIdentifier(tpl.Slug, id.New("atpl"), applicationIdentifierMaxLength-longestSuffix)
	}

	appName := ""
	if in.ApplicationName != nil {
		appName = strings.TrimSpace(*in.ApplicationName)
	}
	installInput := appTemplateInstallInput{
		ApplicationName:           appName,
		ApplicationIdentifier:     baseIdentifier,
		Stage:                     strings.TrimSpace(in.Stage),
		ClusterID:                 clusterID,
		ProjectVolumeID:           resolvedProjectVolumeID,
		ComponentProjectVolumeIDs: resolvedComponentVolumeIDs,
		InstallNow:                in.InstallNow,
		Values:                    mergedValues,
	}

	c, rec := newAIToolGin(tc)
	plan, ok := h.buildTemplateInstallPlan(c, user, project, tpl, installInput)
	if !ok {
		return model.User{}, model.Project{}, appstore.Template{}, templateInstallPlan{}, installTemplateLockedPayload{}, nil,
			aiToolErrorFromRecorder(rec, "app_template.install_invalid", "template installation validation failed")
	}

	nextLock := installTemplateLockedPayload{
		ProjectID:                 project.ID,
		TemplateID:                tpl.ID,
		ClusterID:                 clusterID,
		BaseApplicationIdentifier: baseIdentifier,
		ProjectVolumeID:           resolvedProjectVolumeID,
		ComponentVolumeIDs:        resolvedComponentVolumeIDs,
	}
	return user, project, tpl, plan, nextLock, sensitiveKeys, nil
}

func (h *Handler) registerDeliveryTemplateAITools(reg *aitool.Registry) {
	// 3. delivery_list_templates
	aitool.Register(reg, aitool.Spec[deliveryListTemplatesInput, map[string]any]{
		Name:           "delivery_list_templates",
		Domain:         aitool.DomainDelivery,
		Title:          "分页浏览应用市场模板",
		Summary:        "分页搜索或浏览租户可安装的应用模板目录摘要（自动排除系统组件模板）",
		AliasesZH:      []string{"应用市场", "模板列表", "搜索模板"},
		AliasesEN:      []string{"list templates", "app marketplace"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionApplicationRead)},
		Execute: func(tc *aitool.ToolContext, in deliveryListTemplatesInput) (map[string]any, error) {
			if in.Category != nil && strings.TrimSpace(*in.Category) != "" {
				cat := strings.TrimSpace(*in.Category)
				if !slices.Contains(appstore.AllowedCategories(), cat) {
					return nil, aitool.BadRequest("argument_invalid", fmt.Sprintf("invalid category %q; allowed values: %s", cat, strings.Join(appstore.AllowedCategories(), ", ")))
				}
			}
			sortBy, sortOrder, _, err := normalizeAISort(in.SortBy, in.SortOrder, map[string]string{
				"popularityWeight": "popularity_weight",
				"name":             "name",
				"createdAt":        "created_at",
			}, "popularityWeight", "desc")
			if err != nil {
				return nil, err
			}
			page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)

			templates, err := appstore.CatalogWithPlatform(tc.Context(), tc.DB)
			if err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "app_template.catalog_unavailable", err.Error())
			}

			createdAtByID := map[string]time.Time{}
			if sortBy == "createdAt" && tc.DB != nil {
				var records []model.PlatformAppTemplate
				if err := tc.DB.Select("id", "created_at").Where("enabled = ?", true).Find(&records).Error; err == nil {
					for _, r := range records {
						createdAtByID[r.ID] = r.CreatedAt
					}
				}
			}

			keyword := ""
			if in.Query != nil {
				keyword = strings.ToLower(strings.TrimSpace(*in.Query))
			}
			catFilter := ""
			if in.Category != nil {
				catFilter = strings.ToLower(strings.TrimSpace(*in.Category))
			}

			filtered := make([]appTemplateSummaryResponse, 0, len(templates))
			for _, tpl := range templates {
				if isSystemComponentTemplate(tpl) {
					continue
				}
				if catFilter != "" && strings.ToLower(tpl.Category) != catFilter {
					continue
				}
				if keyword != "" && !templateMatchesQuery(tpl, keyword) {
					continue
				}
				filtered = append(filtered, appTemplateSummaryFrom(tpl))
			}

			asc := sortOrder == "asc"
			sort.SliceStable(filtered, func(i, j int) bool {
				left, right := filtered[i], filtered[j]
				var cmp int
				switch sortBy {
				case "name":
					cmp = strings.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name))
				case "createdAt":
					lt, rt := createdAtByID[left.ID], createdAtByID[right.ID]
					if !lt.Equal(rt) {
						if lt.Before(rt) {
							cmp = -1
						} else {
							cmp = 1
						}
					}
				default: // popularityWeight
					cmp = left.PopularityWeight - right.PopularityWeight
				}
				if cmp == 0 {
					return left.ID < right.ID
				}
				if asc {
					return cmp < 0
				}
				return cmp > 0
			})

			total := int64(len(filtered))
			start := (page - 1) * pageSize
			var paged []appTemplateSummaryResponse
			if start < len(filtered) {
				end := min(start+pageSize, len(filtered))
				paged = filtered[start:end]
			} else {
				paged = []appTemplateSummaryResponse{}
			}
			return aitool.PageEnvelope(paged, page, pageSize, sortBy, sortOrder, total), nil
		},
	})

	// 4. delivery_get_template
	aitool.Register(reg, aitool.Spec[deliveryGetTemplateInput, map[string]any]{
		Name:           "delivery_get_template",
		Domain:         aitool.DomainDelivery,
		Title:          "查看单个应用模板详情",
		Summary:        "读取单个应用模板的普通参数 values、敏感参数 secretValues 及持久化卷要求",
		AliasesZH:      []string{"模板详情", "查看模板参数"},
		AliasesEN:      []string{"get template", "inspect template"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionApplicationRead)},
		Execute: func(tc *aitool.ToolContext, in deliveryGetTemplateInput) (map[string]any, error) {
			tpl, found, err := appstore.FindWithPlatform(tc.Context(), tc.DB, in.Template)
			if err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "app_template.catalog_unavailable", err.Error())
			}
			if !found || isSystemComponentTemplate(tpl) {
				return nil, aitool.NotFound("app_template.not_found", "application template was not found")
			}

			summary := appTemplateSummaryFrom(tpl)
			plainValues := make([]appTemplateValueResponse, 0, len(tpl.Values))
			secretValues := make([]appTemplateValueResponse, 0, len(tpl.Values))
			for _, def := range tpl.Values {
				item := appTemplateValueResponse{
					Key:          def.Key,
					Label:        def.Label,
					Description:  def.Description,
					Default:      def.Default,
					Required:     def.Required,
					Secret:       def.Secret,
					AutoGenerate: def.AutoGenerate,
				}
				if def.Secret || def.AutoGenerate {
					item.Default = ""
					secretValues = append(secretValues, item)
				} else {
					plainValues = append(plainValues, item)
				}
			}

			requiresProjectVolume := false
			for _, dv := range summary.DataVolumes {
				if dv.SourceType == "projectVolume" {
					requiresProjectVolume = true
					break
				}
			}
			componentVolumeKeys := make([]string, 0)
			for _, comp := range summary.Components {
				for _, dv := range comp.DataVolumes {
					if dv.SourceType == "projectVolume" {
						componentVolumeKeys = append(componentVolumeKeys, comp.Key)
						break
					}
				}
			}

			return map[string]any{
				"id":                    summary.ID,
				"slug":                  summary.Slug,
				"name":                  summary.Name,
				"description":           summary.Description,
				"category":              summary.Category,
				"kind":                  summary.Kind,
				"source":                summary.Source,
				"icon":                  summary.Icon,
				"officialWebsite":       summary.OfficialWebsite,
				"officialRepository":    summary.OfficialRepository,
				"popularityWeight":      summary.PopularityWeight,
				"featured":              summary.Featured,
				"isMultiComponent":      tpl.IsMultiComponent(),
				"image":                 summary.Image,
				"version":               summary.Version,
				"servicePort":           summary.ServicePort,
				"defaultReplicas":       summary.DefaultReplicas,
				"defaultCPU":            summary.DefaultCPU,
				"defaultMemory":         summary.DefaultMemory,
				"values":                plainValues,
				"secretValues":          secretValues,
				"dataVolumes":           summary.DataVolumes,
				"requiresProjectVolume": requiresProjectVolume,
				"componentVolumeKeys":   componentVolumeKeys,
				"components":            summary.Components,
			}, nil
		},
	})

	// 8. delivery_install_template
	aitool.Register(reg, aitool.Spec[deliveryInstallTemplateInput, map[string]any]{
		Name:           "delivery_install_template",
		Domain:         aitool.DomainDelivery,
		Title:          "从应用模板安装应用",
		Summary:        "从应用市场模板安装单组件或多组件应用，自动建立组件间依赖并可选触发首次发布",
		AliasesZH:      []string{"安装应用模板", "从应用市场安装", "部署应用模板"},
		AliasesEN:      []string{"install template", "deploy from template"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectAsync,
		RequiredScopes: []string{string(authz.ActionProjectWrite)},
		SensitivePaths: []string{"secretValues.*"},
		Verification: []aitool.OperationVerification{
			{OperationID: "delivery_get_application", Family: "application", Expect: "present"},
		},
		Preview: func(tc *aitool.ToolContext, in deliveryInstallTemplateInput) (*aitool.PreviewResult, error) {
			_, project, tpl, plan, lockPayload, sensitiveKeys, err := h.prepareAITemplateInstall(tc, in, nil)
			if err != nil {
				return nil, err
			}

			installNow := in.InstallNow == nil || *in.InstallNow
			changes := make([]aitool.PreviewChange, 0, len(plan.Components)+2)
			impact := make([]string, 0, len(plan.Components)+3)
			for _, comp := range plan.Components {
				label := comp.Application.Identifier
				if comp.Key != "" {
					label = fmt.Sprintf("%s (%s)", comp.Key, comp.Application.Identifier)
				}
				changes = append(changes, aitool.PreviewChange{
					Field:  "component:" + firstNonEmpty(comp.Key, "main"),
					After:  fmt.Sprintf("%s -> image=%s stage=%s replicas=%d", label, comp.DeploymentTarget.ImageRef, comp.DeploymentTarget.Stage, comp.DeploymentTarget.Replicas),
					Action: "create",
				})
				for _, dv := range comp.DataVolumes {
					if dv.ProjectVolumeID != "" {
						impact = append(impact, fmt.Sprintf("Will exclusively bind ProjectVolume %s to %s (%s)", dv.ProjectVolumeID, comp.Application.Identifier, firstNonEmpty(dv.MountPath, dv.DevicePath)))
					}
				}
			}
			if len(plan.ServiceBindings) > 0 {
				impact = append(impact, fmt.Sprintf("Will create %d internal ServiceBinding(s) between template components", len(plan.ServiceBindings)))
			}
			if installNow {
				impact = append(impact, fmt.Sprintf("Will immediately enqueue %d Release(s) on cluster %s", len(plan.Components), lockPayload.ClusterID))
			} else {
				impact = append(impact, "installNow=false: applications and deployment targets will be created without starting a Release")
			}

			return &aitool.PreviewResult{
				Preview: aitool.ApprovalPreview{
					Summary:        fmt.Sprintf("Install template %s (%s) into project %s stage %s (%d component(s))", tpl.Name, tpl.Slug, project.Identifier, plan.Components[0].DeploymentTarget.Stage, len(plan.Components)),
					TargetResource: "app_template",
					TargetID:       tpl.ID,
					ProjectID:      project.ID,
					Changes:        changes,
					SensitiveKeys:  sensitiveKeys,
					Impact:         impact,
				},
				LockedPayload: lockPayload,
			}, nil
		},
		Execute: func(tc *aitool.ToolContext, in deliveryInstallTemplateInput) (map[string]any, error) {
			var lockPtr *installTemplateLockedPayload
			if len(bytes.TrimSpace(tc.LockedPayload)) > 0 {
				var locked installTemplateLockedPayload
				if err := tc.UnmarshalLockedPayload(&locked); err != nil {
					return nil, aitool.Conflict("ai.preview_lock_invalid", err.Error())
				}
				lockPtr = &locked
			}

			user, project, tpl, plan, _, _, err := h.prepareAITemplateInstall(tc, in, lockPtr)
			if err != nil {
				return nil, err
			}

			mountChanges := make([]deploymentVolumeMountChanges, len(plan.Components))
			if txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
				for index := range plan.Components {
					component := &plan.Components[index]
					if err := createApplicationRecord(tx, &component.Application); err != nil {
						return err
					}
					if err := tx.Create(&component.DeploymentTarget).Error; err != nil {
						return err
					}
					var syncErr error
					mountChanges[index], syncErr = h.host.SyncDeploymentTargetVolumeMounts(tc.Context(), tx, component.DeploymentTarget, component.DataVolumes)
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
				if tpl.IsMultiComponent() {
					for _, component := range plan.Components {
						record := model.AppTemplateInstallationComponent{
							ID:                 id.New("atpc"),
							InstallationID:     plan.Installation.ID,
							ComponentKey:       component.Key,
							ProjectID:          project.ID,
							ApplicationID:      component.Application.ID,
							DeploymentTargetID: component.DeploymentTarget.ID,
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
			}); txErr != nil {
				for _, changes := range mountChanges {
					h.auditDeploymentVolumeMountFailure(tc.Context(), user.ID, changes, txErr)
				}
				h.auditWithContext(user.ID, "app_template.install", plan.Installation.ID, false, appTemplateInstallErrorCode(txErr), tc.Context())
				c, rec := newAIToolGin(tc)
				h.writeTemplateInstallError(c, txErr)
				return nil, aiToolErrorFromRecorder(rec, appTemplateInstallErrorCode(txErr), "template installation failed")
			}

			for _, entry := range plan.SecretValues {
				h.auditWithContext(user.ID, "secret.write", entry.ID, true, entry.Resource, tc.Context())
			}
			for _, binding := range plan.ServiceBindings {
				h.auditWithContext(user.ID, "service_binding.create", binding.ID, true, binding.SourceDeploymentTargetID, tc.Context())
			}
			for index, component := range plan.Components {
				h.auditDeploymentVolumeMountChanges(tc.Context(), user.ID, component.DeploymentTarget, mountChanges[index])
			}
			h.auditWithContext(user.ID, "app_template.install", plan.Installation.ID, true, tpl.ID, tc.Context())

			if len(plan.ServiceBindings) > 0 {
				h.host.EnqueueNetworkPolicyProjectReconcile(tc.Context(), project.ID)
			}

			for _, component := range plan.Components {
				if component.Release == nil || h.enqueueDeployRun(tc.Context(), *component.Release) {
					continue
				}
				if err := markTemplateReleaseDispatchFailed(tc.DB, component.Release); err != nil {
					return nil, aitool.NewError(http.StatusInternalServerError, "app_template.release_dispatch_failed", err.Error())
				}
				h.auditWithContext(user.ID, "app_template.deploy_enqueue", plan.Installation.ID, false, model.ReleaseFailureDispatchFailed, tc.Context())
			}

			componentsOut := make([]aiInstalledComponentSummary, 0, len(plan.Components))
			for _, component := range plan.Components {
				compSummary := aiInstalledComponentSummary{
					Key:                   firstNonEmpty(component.Key, "main"),
					ApplicationID:         component.Application.ID,
					ApplicationIdentifier: component.Application.Identifier,
					ApplicationName:       component.Application.Name,
					DeploymentTargetID:    component.DeploymentTarget.ID,
				}
				if component.Release != nil {
					compSummary.ReleaseID = component.Release.ID
				}
				componentsOut = append(componentsOut, compSummary)
			}

			entry := plan.Components[0]
			installNow := entry.Release != nil
			out := map[string]any{
				"id":                        entry.Application.ID,
				"projectId":                 project.ID,
				"installationId":            plan.Installation.ID,
				"templateId":                tpl.ID,
				"templateSlug":              tpl.Slug,
				"stage":                     entry.DeploymentTarget.Stage,
				"applicationId":             entry.Application.ID,
				"primaryApplicationId":      entry.Application.ID,
				"primaryDeploymentTargetId": entry.DeploymentTarget.ID,
				"requiresRelease":           !installNow,
				"components":                componentsOut,
			}
			if entry.Release != nil {
				out["primaryReleaseId"] = entry.Release.ID
				out["releaseId"] = entry.Release.ID
			}
			return out, nil
		},
	})
}

// --- Admin Platform App Template AI Tools (3) ---

type adminListPlatformAppTemplatesInput struct {
	Template       *string `json:"template,omitempty" desc:"按平台模板 ID 或 slug 在摘要列表模式下精确过滤单条记录"`
	Query          *string `json:"query,omitempty" desc:"按名称、标识或简介关键词搜索"`
	Category       *string `json:"category,omitempty" enum:"collaboration|database|developerTool|middleware|observability|security|storage" desc:"按分类过滤"`
	Enabled        *bool   `json:"enabled,omitempty" desc:"按上架/下架状态过滤"`
	ExportTemplate *string `json:"exportTemplate,omitempty" desc:"传入内置或平台模板 ID/slug 单查导出完整 luna.app-template/v1 Bundle"`
	Page           *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码"`
	PageSize       *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（上限 50）"`
}

type aiPlatformTemplateComponentSummary struct {
	Key             string                `json:"key"`
	Name            string                `json:"name"`
	Image           string                `json:"image"`
	ServicePort     int                   `json:"servicePort"`
	DefaultReplicas int                   `json:"defaultReplicas"`
	DefaultCPU      string                `json:"defaultCPU"`
	DefaultMemory   string                `json:"defaultMemory"`
	DataVolumes     []appstore.DataVolume `json:"dataVolumes"`
	EnvKeys         []string              `json:"envKeys"`
	SecretEnvKeys   []string              `json:"secretEnvKeys"`
	ConfigFilePaths []string              `json:"configFilePaths"`
	SecretFilePaths []string              `json:"secretFilePaths"`
	Dependencies    []string              `json:"dependencies"`
}

type aiPlatformAppTemplateSummaryItem struct {
	ID                 string                               `json:"id"`
	Slug               string                               `json:"slug"`
	Name               string                               `json:"name"`
	Description        string                               `json:"description"`
	Category           string                               `json:"category"`
	Icon               string                               `json:"icon"`
	Version            string                               `json:"version"`
	IsMultiComponent   bool                                 `json:"isMultiComponent"`
	Enabled            bool                                 `json:"enabled"`
	Featured           bool                                 `json:"featured"`
	PopularityWeight   int                                  `json:"popularityWeight"`
	OfficialWebsite    string                               `json:"officialWebsite"`
	OfficialRepository string                               `json:"officialRepository"`
	Image              string                               `json:"image"`
	ServicePort        int                                  `json:"servicePort"`
	DefaultReplicas    int                                  `json:"defaultReplicas"`
	DefaultCPU         string                               `json:"defaultCPU"`
	DefaultMemory      string                               `json:"defaultMemory"`
	ContainerCommand   string                               `json:"containerCommand"`
	ContainerArgs      string                               `json:"containerArgs"`
	FSGroup            string                               `json:"fsGroup"`
	DataVolumes        []appstore.DataVolume                `json:"dataVolumes"`
	EnvKeys            []string                             `json:"envKeys"`
	SecretEnvKeys      []string                             `json:"secretEnvKeys"`
	ConfigFilePaths    []string                             `json:"configFilePaths"`
	SecretFilePaths    []string                             `json:"secretFilePaths"`
	Components         []aiPlatformTemplateComponentSummary `json:"components"`
	Values             []appTemplateValueResponse           `json:"values"`
	CreatedBy          string                               `json:"createdBy"`
	CreatedAt          time.Time                            `json:"createdAt"`
	UpdatedAt          time.Time                            `json:"updatedAt"`
}

func sortedMapKeys(m map[string]string) []string {
	if len(m) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func configFilePathsOf(files []appstore.ConfigFile) []string {
	if len(files) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	sort.Strings(out)
	return out
}

func platformTemplateSummaryItemFrom(record model.PlatformAppTemplate) (aiPlatformAppTemplateSummaryItem, appstore.Template, error) {
	detail, err := platformAppTemplateResponseFrom(record)
	if err != nil {
		return aiPlatformAppTemplateSummaryItem{}, appstore.Template{}, err
	}
	tpl, err := appstore.TemplateFromPlatformModel(record)
	if err != nil {
		return aiPlatformAppTemplateSummaryItem{}, appstore.Template{}, err
	}
	comps := make([]aiPlatformTemplateComponentSummary, 0, len(detail.Components))
	for _, c := range detail.Components {
		comps = append(comps, aiPlatformTemplateComponentSummary{
			Key:             c.Key,
			Name:            c.Name,
			Image:           c.Image,
			ServicePort:     c.ServicePort,
			DefaultReplicas: c.DefaultReplicas,
			DefaultCPU:      c.DefaultCPU,
			DefaultMemory:   c.DefaultMemory,
			DataVolumes:     c.DataVolumes,
			EnvKeys:         sortedMapKeys(c.Env),
			SecretEnvKeys:   sortedMapKeys(c.SecretEnv),
			ConfigFilePaths: configFilePathsOf(c.ConfigFiles),
			SecretFilePaths: configFilePathsOf(c.SecretFiles),
			Dependencies:    c.Dependencies,
		})
	}
	return aiPlatformAppTemplateSummaryItem{
		ID:                 detail.ID,
		Slug:               detail.Slug,
		Name:               detail.Name,
		Description:        detail.Description,
		Category:           detail.Category,
		Icon:               detail.Icon,
		Version:            detail.Version,
		IsMultiComponent:   detail.IsMultiComponent,
		Enabled:            detail.Enabled,
		Featured:           detail.Featured,
		PopularityWeight:   detail.PopularityWeight,
		OfficialWebsite:    detail.OfficialWebsite,
		OfficialRepository: detail.OfficialRepository,
		Image:              detail.Image,
		ServicePort:        detail.ServicePort,
		DefaultReplicas:    detail.DefaultReplicas,
		DefaultCPU:         detail.DefaultCPU,
		DefaultMemory:      detail.DefaultMemory,
		ContainerCommand:   detail.ContainerCommand,
		ContainerArgs:      detail.ContainerArgs,
		FSGroup:            detail.FSGroup,
		DataVolumes:        detail.DataVolumes,
		EnvKeys:            sortedMapKeys(detail.Env),
		SecretEnvKeys:      sortedMapKeys(detail.SecretEnv),
		ConfigFilePaths:    configFilePathsOf(detail.ConfigFiles),
		SecretFilePaths:    configFilePathsOf(detail.SecretFiles),
		Components:         comps,
		Values:             detail.Values,
		CreatedBy:          detail.CreatedBy,
		CreatedAt:          detail.CreatedAt,
		UpdatedAt:          detail.UpdatedAt,
	}, tpl, nil
}

type aiPlatformTemplateEmptyDirPatch struct {
	Medium    *string `json:"medium,omitempty" desc:"EmptyDir 存储介质（如 Memory）"`
	SizeLimit *string `json:"sizeLimit,omitempty" desc:"EmptyDir 容量上限（如 256Mi）"`
}

type aiPlatformTemplateDataVolumePatch struct {
	LogicalName string                           `json:"logicalName" desc:"逻辑卷名"`
	SourceType  string                           `json:"sourceType" enum:"projectVolume|emptyDir" desc:"数据卷来源类型"`
	MountPath   *string                          `json:"mountPath,omitempty" desc:"Filesystem 模式挂载路径"`
	DevicePath  *string                          `json:"devicePath,omitempty" desc:"Block 模式设备路径"`
	ReadOnly    *bool                            `json:"readOnly,omitempty" desc:"是否只读挂载"`
	EmptyDir    *aiPlatformTemplateEmptyDirPatch `json:"emptyDir,omitempty" desc:"EmptyDir 配置"`
}

type aiPlatformTemplateWorkloadPatch struct {
	Image                 *string                             `json:"image,omitempty" desc:"容器镜像地址"`
	ServicePort           *int                                `json:"servicePort,omitempty" min:"1" max:"65535" desc:"主服务端口"`
	DefaultReplicas       *int                                `json:"defaultReplicas,omitempty" min:"1" desc:"默认副本数"`
	DefaultCPU            *string                             `json:"defaultCPU,omitempty" desc:"默认 CPU 请求"`
	DefaultMemory         *string                             `json:"defaultMemory,omitempty" desc:"默认内存请求"`
	ContainerCommand      *string                             `json:"containerCommand,omitempty" desc:"容器启动命令覆盖"`
	ClearContainerCommand *bool                               `json:"clearContainerCommand,omitempty" desc:"清空容器启动命令"`
	ContainerArgs         *string                             `json:"containerArgs,omitempty" desc:"容器启动参数覆盖"`
	ClearContainerArgs    *bool                               `json:"clearContainerArgs,omitempty" desc:"清空容器启动参数"`
	FSGroup               *string                             `json:"fsGroup,omitempty" desc:"Pod FSGroup"`
	ClearFSGroup          *bool                               `json:"clearFsGroup,omitempty" desc:"清空 FSGroup"`
	UpsertDataVolumes     []aiPlatformTemplateDataVolumePatch `json:"upsertDataVolumes,omitempty" desc:"按 logicalName 增量新增或更新数据卷声明"`
	RemoveDataVolumeNames []string                            `json:"removeDataVolumeNames,omitempty" desc:"按 logicalName 移除数据卷声明"`
	SetEnv                map[string]string                   `json:"setEnv,omitempty" desc:"增量设置普通环境变量"`
	RemoveEnvKeys         []string                            `json:"removeEnvKeys,omitempty" desc:"移除指定普通环境变量"`
	SetSecretEnv          map[string]string                   `json:"setSecretEnv,omitempty" desc:"增量设置敏感环境变量模板（仅允许 {{PLACEHOLDER}} 占位符引用，严禁明文字面量）"`
	RemoveSecretEnvKeys   []string                            `json:"removeSecretEnvKeys,omitempty" desc:"移除指定敏感环境变量"`
	UpsertConfigFiles     []appstore.ConfigFile               `json:"upsertConfigFiles,omitempty" desc:"按 path 增量新增或更新明文配置文件"`
	RemoveConfigFilePaths []string                            `json:"removeConfigFilePaths,omitempty" desc:"按 path 移除明文配置文件"`
	UpsertSecretFiles     []appstore.ConfigFile               `json:"upsertSecretFiles,omitempty" desc:"按 path 增量新增或更新敏感配置文件模板（仅允许 {{PLACEHOLDER}} 占位符引用）"`
	RemoveSecretFilePaths []string                            `json:"removeSecretFilePaths,omitempty" desc:"按 path 移除敏感配置文件"`
}

type aiPlatformTemplateComponentPatch struct {
	Key      string                           `json:"key" pattern:"^[a-z][a-z0-9]*$" desc:"组件唯一标识（首元素为入口组件）"`
	Name     *string                          `json:"name,omitempty" desc:"组件显示名称"`
	Workload *aiPlatformTemplateWorkloadPatch `json:"workload,omitempty" desc:"组件工作负载定义或增量补丁"`
}

type aiPlatformTemplateValuePatch struct {
	Key          string  `json:"key" desc:"参数键名"`
	Label        *string `json:"label,omitempty" desc:"参数标签名"`
	Description  *string `json:"description,omitempty" desc:"参数说明"`
	Default      *string `json:"default,omitempty" desc:"默认值（secret=true 时禁止设置明文默认值）"`
	ClearDefault *bool   `json:"clearDefault,omitempty" desc:"清空默认值"`
	Required     *bool   `json:"required,omitempty" desc:"是否必填"`
	Secret       *bool   `json:"secret,omitempty" desc:"是否为敏感参数"`
	AutoGenerate *bool   `json:"autoGenerate,omitempty" desc:"省略时是否自动生成随机密钥"`
}

type adminSavePlatformAppTemplateInput struct {
	Template                *string                            `json:"template,omitempty" desc:"更新现有平台模板时传入模板 ID 或 slug"`
	ImportBundle            map[string]any                     `json:"importBundle,omitempty" desc:"直接导入 luna.app-template/v1 标准 Bundle（可叠加下方增量修改字段）"`
	Overwrite               *bool                              `json:"overwrite,omitempty" default:"false" desc:"配合 importBundle 使用，是否覆盖同 slug 平台模板"`
	Slug                    *string                            `json:"slug,omitempty" desc:"平台模板唯一标识（2..63 位小写字母数字连字符，禁占内置模板 slug）"`
	Name                    *string                            `json:"name,omitempty" desc:"模板显示名称"`
	Description             *string                            `json:"description,omitempty" desc:"模板简介"`
	Category                *string                            `json:"category,omitempty" enum:"collaboration|database|developerTool|middleware|observability|security|storage" desc:"模板分类"`
	Icon                    *string                            `json:"icon,omitempty" max:"64" desc:"模板图标"`
	Version                 *string                            `json:"version,omitempty" desc:"模板版本号（新建默认 1.0.0）"`
	Enabled                 *bool                              `json:"enabled,omitempty" desc:"是否上架启用"`
	Featured                *bool                              `json:"featured,omitempty" desc:"是否设为推荐模板"`
	PopularityWeight        *int                               `json:"popularityWeight,omitempty" min:"0" max:"1000" desc:"热度排序权重（0..1000）"`
	OfficialWebsite         *string                            `json:"officialWebsite,omitempty" desc:"官方网站 URL"`
	ClearOfficialWebsite    *bool                              `json:"clearOfficialWebsite,omitempty" desc:"清空官方网站 URL"`
	OfficialRepository      *string                            `json:"officialRepository,omitempty" desc:"官方代码仓库 URL"`
	ClearOfficialRepository *bool                              `json:"clearOfficialRepository,omitempty" desc:"清空官方代码仓库 URL"`
	Workload                *aiPlatformTemplateWorkloadPatch   `json:"workload,omitempty" desc:"单组件模板工作负载定义或增量补丁（与 upsertComponents 互斥）"`
	UpsertComponents        []aiPlatformTemplateComponentPatch `json:"upsertComponents,omitempty" desc:"多组件模板按 key 新增或增量更新组件"`
	RemoveComponentKeys     []string                           `json:"removeComponentKeys,omitempty" desc:"多组件模板按 key 移除组件"`
	UpsertValues            []aiPlatformTemplateValuePatch     `json:"upsertValues,omitempty" desc:"按 key 增量新增或更新模板参数定义"`
	RemoveValueKeys         []string                           `json:"removeValueKeys,omitempty" desc:"按 key 移除模板参数定义"`
}

type adminDeletePlatformAppTemplateInput struct {
	Template string `json:"template" desc:"平台自定义模板 ID 或 slug"`
}

type platformTemplateLockedPayload struct {
	Mode       string `json:"mode"`
	ExistingID string `json:"existingId,omitempty"`
	Slug       string `json:"slug"`
}

func applyAIWorkloadPatch(base appstore.Workload, patch aiPlatformTemplateWorkloadPatch) (appstore.Workload, error) {
	if patch.ClearContainerCommand != nil && *patch.ClearContainerCommand && patch.ContainerCommand != nil && strings.TrimSpace(*patch.ContainerCommand) != "" {
		return appstore.Workload{}, aitool.BadRequest("argument_invalid", "cannot specify both containerCommand and clearContainerCommand=true")
	}
	if patch.ClearContainerArgs != nil && *patch.ClearContainerArgs && patch.ContainerArgs != nil && strings.TrimSpace(*patch.ContainerArgs) != "" {
		return appstore.Workload{}, aitool.BadRequest("argument_invalid", "cannot specify both containerArgs and clearContainerArgs=true")
	}
	if patch.ClearFSGroup != nil && *patch.ClearFSGroup && patch.FSGroup != nil && strings.TrimSpace(*patch.FSGroup) != "" {
		return appstore.Workload{}, aitool.BadRequest("argument_invalid", "cannot specify both fsGroup and clearFsGroup=true")
	}

	out := base
	if patch.Image != nil {
		out.Image = strings.TrimSpace(*patch.Image)
	}
	if patch.ServicePort != nil {
		out.ServicePort = *patch.ServicePort
	}
	if patch.DefaultReplicas != nil {
		out.DefaultReplicas = *patch.DefaultReplicas
	}
	if patch.DefaultCPU != nil {
		out.DefaultCPU = strings.TrimSpace(*patch.DefaultCPU)
	}
	if patch.DefaultMemory != nil {
		out.DefaultMemory = strings.TrimSpace(*patch.DefaultMemory)
	}
	if patch.ClearContainerCommand != nil && *patch.ClearContainerCommand {
		out.ContainerCommand = ""
	} else if patch.ContainerCommand != nil {
		out.ContainerCommand = strings.TrimSpace(*patch.ContainerCommand)
	}
	if patch.ClearContainerArgs != nil && *patch.ClearContainerArgs {
		out.ContainerArgs = ""
	} else if patch.ContainerArgs != nil {
		out.ContainerArgs = strings.TrimSpace(*patch.ContainerArgs)
	}
	if patch.ClearFSGroup != nil && *patch.ClearFSGroup {
		out.FSGroup = ""
	} else if patch.FSGroup != nil {
		out.FSGroup = strings.TrimSpace(*patch.FSGroup)
	}

	// DataVolumes
	if len(patch.UpsertDataVolumes) > 0 || len(patch.RemoveDataVolumeNames) > 0 {
		removeSet := make(map[string]bool, len(patch.RemoveDataVolumeNames))
		for _, n := range patch.RemoveDataVolumeNames {
			removeSet[strings.TrimSpace(n)] = true
		}
		dvs := make([]appstore.DataVolume, 0, len(out.DataVolumes)+len(patch.UpsertDataVolumes))
		indexByName := map[string]int{}
		for _, existing := range out.DataVolumes {
			if removeSet[strings.TrimSpace(existing.LogicalName)] {
				continue
			}
			indexByName[strings.TrimSpace(existing.LogicalName)] = len(dvs)
			dvs = append(dvs, existing)
		}
		for _, up := range patch.UpsertDataVolumes {
			ln := strings.TrimSpace(up.LogicalName)
			if removeSet[ln] {
				return appstore.Workload{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("dataVolume %q cannot be in both upsertDataVolumes and removeDataVolumeNames", ln))
			}
			dv := appstore.DataVolume{
				LogicalName: ln,
				SourceType:  strings.TrimSpace(up.SourceType),
			}
			if up.MountPath != nil {
				dv.MountPath = strings.TrimSpace(*up.MountPath)
			}
			if up.DevicePath != nil {
				dv.DevicePath = strings.TrimSpace(*up.DevicePath)
			}
			if up.ReadOnly != nil {
				dv.ReadOnly = *up.ReadOnly
			}
			if up.EmptyDir != nil {
				dv.EmptyDir = &appstore.EmptyDirSpec{}
				if up.EmptyDir.Medium != nil {
					dv.EmptyDir.Medium = strings.TrimSpace(*up.EmptyDir.Medium)
				}
				if up.EmptyDir.SizeLimit != nil {
					dv.EmptyDir.SizeLimit = strings.TrimSpace(*up.EmptyDir.SizeLimit)
				}
			}
			if idx, ok := indexByName[ln]; ok {
				dvs[idx] = dv
			} else {
				indexByName[ln] = len(dvs)
				dvs = append(dvs, dv)
			}
		}
		out.DataVolumes = dvs
	}

	// Env & SecretEnv
	if len(patch.SetEnv) > 0 || len(patch.RemoveEnvKeys) > 0 {
		env := maps.Clone(nonNilStringMap(out.Env))
		for _, k := range patch.RemoveEnvKeys {
			delete(env, strings.TrimSpace(k))
		}
		for k, v := range patch.SetEnv {
			env[strings.TrimSpace(k)] = v
		}
		if len(env) == 0 {
			out.Env = nil
		} else {
			out.Env = env
		}
	}
	if len(patch.SetSecretEnv) > 0 || len(patch.RemoveSecretEnvKeys) > 0 {
		senv := maps.Clone(nonNilStringMap(out.SecretEnv))
		for _, k := range patch.RemoveSecretEnvKeys {
			delete(senv, strings.TrimSpace(k))
		}
		for k, v := range patch.SetSecretEnv {
			senv[strings.TrimSpace(k)] = v
		}
		if len(senv) == 0 {
			out.SecretEnv = nil
		} else {
			out.SecretEnv = senv
		}
	}

	// ConfigFiles & SecretFiles
	mergeFiles := func(existing []appstore.ConfigFile, upserts []appstore.ConfigFile, removes []string) []appstore.ConfigFile {
		if len(upserts) == 0 && len(removes) == 0 {
			return existing
		}
		rem := make(map[string]bool, len(removes))
		for _, p := range removes {
			cleaned := path.Clean(strings.TrimSpace(p))
			rem[cleaned] = true
			rem[strings.TrimSpace(p)] = true
		}
		res := make([]appstore.ConfigFile, 0, len(existing)+len(upserts))
		idxByPath := map[string]int{}
		for _, f := range existing {
			cleaned := path.Clean(strings.TrimSpace(f.Path))
			if rem[cleaned] || rem[strings.TrimSpace(f.Path)] {
				continue
			}
			idxByPath[cleaned] = len(res)
			res = append(res, f)
		}
		for _, u := range upserts {
			cleaned := path.Clean(strings.TrimSpace(u.Path))
			entry := appstore.ConfigFile{Path: strings.TrimSpace(u.Path), Content: u.Content}
			if idx, ok := idxByPath[cleaned]; ok {
				res[idx] = entry
			} else {
				idxByPath[cleaned] = len(res)
				res = append(res, entry)
			}
		}
		return res
	}
	out.ConfigFiles = mergeFiles(out.ConfigFiles, patch.UpsertConfigFiles, patch.RemoveConfigFilePaths)
	out.SecretFiles = mergeFiles(out.SecretFiles, patch.UpsertSecretFiles, patch.RemoveSecretFilePaths)

	return out, nil
}

func applyAIValuePatches(existing []appstore.ValueDefinition, upserts []aiPlatformTemplateValuePatch, removeKeys []string) ([]appstore.ValueDefinition, error) {
	if len(upserts) == 0 && len(removeKeys) == 0 {
		return existing, nil
	}
	removeSet := make(map[string]bool, len(removeKeys))
	for _, k := range removeKeys {
		removeSet[strings.TrimSpace(k)] = true
	}
	out := make([]appstore.ValueDefinition, 0, len(existing)+len(upserts))
	idxByKey := map[string]int{}
	for _, item := range existing {
		k := strings.TrimSpace(item.Key)
		if removeSet[k] {
			continue
		}
		idxByKey[k] = len(out)
		out = append(out, item)
	}
	for _, p := range upserts {
		key := strings.TrimSpace(p.Key)
		if removeSet[key] {
			return nil, aitool.BadRequest("argument_invalid", fmt.Sprintf("value key %q cannot be in both upsertValues and removeValueKeys", key))
		}
		if p.ClearDefault != nil && *p.ClearDefault && p.Default != nil && strings.TrimSpace(*p.Default) != "" {
			return nil, aitool.BadRequest("argument_invalid", fmt.Sprintf("value %q cannot specify both default and clearDefault=true", key))
		}
		var def appstore.ValueDefinition
		idx, found := idxByKey[key]
		if found {
			def = out[idx]
		} else {
			def = appstore.ValueDefinition{Key: key}
		}
		if p.Label != nil {
			def.Label = strings.TrimSpace(*p.Label)
		}
		if p.Description != nil {
			def.Description = strings.TrimSpace(*p.Description)
		}
		if p.Required != nil {
			def.Required = *p.Required
		}
		if p.Secret != nil {
			def.Secret = *p.Secret
		}
		if p.AutoGenerate != nil {
			def.AutoGenerate = *p.AutoGenerate
		}
		if p.ClearDefault != nil && *p.ClearDefault {
			def.Default = ""
		} else if p.Default != nil {
			if def.Secret && strings.TrimSpace(*p.Default) != "" {
				return nil, aitool.BadRequest("app_template.secret_default_forbidden", fmt.Sprintf("secret value %q must not contain a plaintext default", key))
			}
			def.Default = strings.TrimSpace(*p.Default)
		} else if def.Secret {
			// Switching an existing parameter to secret=true automatically clears any legacy plaintext default (catalog §5.2 #26).
			def.Default = ""
		}
		if found {
			out[idx] = def
		} else {
			idxByKey[key] = len(out)
			out = append(out, def)
		}
	}
	return out, nil
}

func mapPlatformValidationErr(err error) error {
	var validationErr *appstore.PlatformTemplateValidationError
	if errors.As(err, &validationErr) {
		code := strings.TrimSpace(validationErr.Code)
		if code == "" {
			code = "app_template.invalid"
		}
		return aitool.BadRequest(code, validationErr.Message)
	}
	return aitool.NewError(http.StatusInternalServerError, "app_template.save_failed", err.Error())
}

func (h *Handler) prepareAIPlatformTemplateSave(
	tc *aitool.ToolContext,
	user model.User,
	in adminSavePlatformAppTemplateInput,
) (string, *model.PlatformAppTemplate, model.PlatformAppTemplate, error) {
	if in.Workload != nil && (len(in.UpsertComponents) > 0 || len(in.RemoveComponentKeys) > 0) {
		return "", nil, model.PlatformAppTemplate{}, aitool.BadRequest("argument_invalid", "workload and upsertComponents/removeComponentKeys are mutually exclusive")
	}
	if in.ClearOfficialWebsite != nil && *in.ClearOfficialWebsite && in.OfficialWebsite != nil && strings.TrimSpace(*in.OfficialWebsite) != "" {
		return "", nil, model.PlatformAppTemplate{}, aitool.BadRequest("argument_invalid", "cannot specify both officialWebsite and clearOfficialWebsite=true")
	}
	if in.ClearOfficialRepository != nil && *in.ClearOfficialRepository && in.OfficialRepository != nil && strings.TrimSpace(*in.OfficialRepository) != "" {
		return "", nil, model.PlatformAppTemplate{}, aitool.BadRequest("argument_invalid", "cannot specify both officialRepository and clearOfficialRepository=true")
	}

	db := tc.DB
	var existingRecord *model.PlatformAppTemplate
	var baseTemplate appstore.Template
	enabled := true
	mode := "create"

	if len(in.ImportBundle) > 0 {
		mode = "import"
		rawBundle, err := json.Marshal(in.ImportBundle)
		if err != nil {
			return "", nil, model.PlatformAppTemplate{}, aitool.BadRequest("argument_invalid", "invalid importBundle JSON: "+err.Error())
		}
		var bundle appstore.PlatformTemplateBundle
		dec := json.NewDecoder(bytes.NewReader(rawBundle))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&bundle); err != nil {
			return "", nil, model.PlatformAppTemplate{}, aitool.BadRequest("argument_invalid", "invalid importBundle schema: "+err.Error())
		}
		if ver := strings.TrimSpace(bundle.SchemaVersion); ver != "" && ver != appstore.PlatformTemplateBundleSchemaVersion {
			return "", nil, model.PlatformAppTemplate{}, aitool.BadRequest("app_template.schema_version_unsupported", fmt.Sprintf("unsupported bundle schemaVersion %q", ver))
		}
		if bundle.Enabled != nil {
			enabled = *bundle.Enabled
		}
		baseTemplate = bundle.ToTemplate("")

		if in.Template != nil && strings.TrimSpace(*in.Template) != "" {
			rec, found, err := findPlatformTemplateRecord(db, *in.Template)
			if err != nil {
				return "", nil, model.PlatformAppTemplate{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}
			if !found {
				return "", nil, model.PlatformAppTemplate{}, aitool.NotFound("app_template.not_found", "platform app template was not found")
			}
			existingRecord = &rec
		}
	} else if in.Template != nil && strings.TrimSpace(*in.Template) != "" {
		mode = "update"
		rec, found, err := findPlatformTemplateRecord(db, *in.Template)
		if err != nil {
			return "", nil, model.PlatformAppTemplate{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if !found {
			return "", nil, model.PlatformAppTemplate{}, aitool.NotFound("app_template.not_found", "platform app template was not found")
		}
		existingRecord = &rec
		enabled = rec.Enabled
		tpl, err := appstore.TemplateFromPlatformModel(rec)
		if err != nil {
			return "", nil, model.PlatformAppTemplate{}, aitool.NewError(http.StatusInternalServerError, "app_template.decode_failed", err.Error())
		}
		baseTemplate = tpl
	} else {
		if in.Slug == nil || strings.TrimSpace(*in.Slug) == "" {
			return "", nil, model.PlatformAppTemplate{}, aitool.BadRequest("argument_invalid", "slug is required when creating a platform app template")
		}
		if in.Name == nil || strings.TrimSpace(*in.Name) == "" {
			return "", nil, model.PlatformAppTemplate{}, aitool.BadRequest("argument_invalid", "name is required when creating a platform app template")
		}
		if in.Category == nil || strings.TrimSpace(*in.Category) == "" {
			return "", nil, model.PlatformAppTemplate{}, aitool.BadRequest("argument_invalid", "category is required when creating a platform app template")
		}
		if in.Workload == nil && len(in.UpsertComponents) == 0 {
			return "", nil, model.PlatformAppTemplate{}, aitool.BadRequest("argument_invalid", "either workload (single-component) or upsertComponents (multi-component) is required when creating a template")
		}
		baseTemplate = appstore.Template{
			Kind:    "application",
			Source:  appstore.TemplateSourcePlatform,
			Version: "1.0.0",
		}
	}

	// Apply scalar patches
	if in.Slug != nil {
		baseTemplate.Slug = strings.ToLower(strings.TrimSpace(*in.Slug))
	}
	if in.Name != nil {
		baseTemplate.Name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		baseTemplate.Description = strings.TrimSpace(*in.Description)
	}
	if in.Category != nil {
		baseTemplate.Category = strings.TrimSpace(*in.Category)
	}
	if in.Icon != nil {
		baseTemplate.Icon = strings.TrimSpace(*in.Icon)
	}
	if in.Version != nil {
		baseTemplate.Version = strings.TrimSpace(*in.Version)
	}
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	if in.Featured != nil {
		baseTemplate.Featured = *in.Featured
	}
	if in.PopularityWeight != nil {
		baseTemplate.PopularityWeight = *in.PopularityWeight
	}
	if in.ClearOfficialWebsite != nil && *in.ClearOfficialWebsite {
		baseTemplate.OfficialWebsite = ""
	} else if in.OfficialWebsite != nil {
		baseTemplate.OfficialWebsite = strings.TrimSpace(*in.OfficialWebsite)
	}
	if in.ClearOfficialRepository != nil && *in.ClearOfficialRepository {
		baseTemplate.OfficialRepository = ""
	} else if in.OfficialRepository != nil {
		baseTemplate.OfficialRepository = strings.TrimSpace(*in.OfficialRepository)
	}

	// Apply workload or component patches
	if in.Workload != nil {
		if baseTemplate.IsMultiComponent() {
			return "", nil, model.PlatformAppTemplate{}, aitool.BadRequest("argument_invalid", "template is multi-component; use upsertComponents to patch component workloads")
		}
		patchedWorkload, err := applyAIWorkloadPatch(baseTemplate.Workload, *in.Workload)
		if err != nil {
			return "", nil, model.PlatformAppTemplate{}, err
		}
		baseTemplate.Workload = patchedWorkload
	}
	if len(in.UpsertComponents) > 0 || len(in.RemoveComponentKeys) > 0 {
		if !baseTemplate.IsMultiComponent() && existingRecord != nil && len(in.ImportBundle) == 0 {
			return "", nil, model.PlatformAppTemplate{}, aitool.BadRequest("argument_invalid", "template is single-component; use workload to patch its workload or importBundle to replace structure")
		}
		removeSet := make(map[string]bool, len(in.RemoveComponentKeys))
		for _, k := range in.RemoveComponentKeys {
			removeSet[strings.TrimSpace(k)] = true
		}
		comps := make([]appstore.Component, 0, len(baseTemplate.Components)+len(in.UpsertComponents))
		idxByKey := map[string]int{}
		for _, c := range baseTemplate.Components {
			k := strings.TrimSpace(c.Key)
			if removeSet[k] {
				continue
			}
			idxByKey[k] = len(comps)
			comps = append(comps, c)
		}
		for _, p := range in.UpsertComponents {
			key := strings.TrimSpace(p.Key)
			if removeSet[key] {
				return "", nil, model.PlatformAppTemplate{}, aitool.BadRequest("argument_invalid", fmt.Sprintf("component %q cannot be in both upsertComponents and removeComponentKeys", key))
			}
			var comp appstore.Component
			idx, found := idxByKey[key]
			if found {
				comp = comps[idx]
			} else {
				comp = appstore.Component{Key: key, Name: key}
			}
			if p.Name != nil && strings.TrimSpace(*p.Name) != "" {
				comp.Name = strings.TrimSpace(*p.Name)
			}
			if p.Workload != nil {
				wl, err := applyAIWorkloadPatch(comp.Workload, *p.Workload)
				if err != nil {
					return "", nil, model.PlatformAppTemplate{}, err
				}
				comp.Workload = wl
			}
			if found {
				comps[idx] = comp
			} else {
				idxByKey[key] = len(comps)
				comps = append(comps, comp)
			}
		}
		baseTemplate.Components = comps
		if len(comps) > 0 {
			baseTemplate.Workload = appstore.Workload{}
		}
	}

	// Apply value definition patches
	patchedValues, err := applyAIValuePatches(baseTemplate.Values, in.UpsertValues, in.RemoveValueKeys)
	if err != nil {
		return "", nil, model.PlatformAppTemplate{}, err
	}
	baseTemplate.Values = patchedValues

	candidateSlug := strings.ToLower(strings.TrimSpace(baseTemplate.Slug))
	reserved, err := appstore.IsBuiltinSlugOrID(candidateSlug)
	if err != nil {
		return "", nil, model.PlatformAppTemplate{}, aitool.NewError(http.StatusInternalServerError, "app_template.catalog_unavailable", err.Error())
	}
	if reserved {
		return "", nil, model.PlatformAppTemplate{}, aitool.Conflict("app_template.slug_reserved", fmt.Sprintf("template slug %q is reserved by a built-in template", candidateSlug))
	}

	if mode == "import" && existingRecord == nil && db != nil {
		rec, found, err := findPlatformTemplateRecord(db, candidateSlug)
		if err != nil {
			return "", nil, model.PlatformAppTemplate{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if found {
			overwrite := in.Overwrite != nil && *in.Overwrite
			if !overwrite {
				return "", nil, model.PlatformAppTemplate{}, aitool.NewErrorWithHint(
					http.StatusConflict,
					"app_template.slug_exists",
					fmt.Sprintf("platform template with slug %q already exists", candidateSlug),
					"Set overwrite=true to replace the existing platform template, or specify a new slug.",
				)
			}
			existingRecord = &rec
		}
	}

	targetID := id.New("ptpl")
	createdBy := user.ID
	if existingRecord != nil {
		targetID = existingRecord.ID
		createdBy = existingRecord.CreatedBy
	}

	record, _, err := appstore.ModelFromPlatformTemplate(targetID, createdBy, enabled, baseTemplate)
	if err != nil {
		return "", nil, model.PlatformAppTemplate{}, mapPlatformValidationErr(err)
	}

	if db != nil {
		var conflictCount int64
		q := db.Model(&model.PlatformAppTemplate{}).Where("slug = ?", record.Slug)
		if existingRecord != nil {
			q = q.Where("id <> ?", existingRecord.ID)
		}
		if err := q.Count(&conflictCount).Error; err != nil {
			return "", nil, model.PlatformAppTemplate{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if conflictCount > 0 {
			return "", nil, model.PlatformAppTemplate{}, aitool.Conflict("app_template.slug_exists", fmt.Sprintf("platform template slug %q already exists", record.Slug))
		}
	}

	if existingRecord != nil {
		record.CreatedAt = existingRecord.CreatedAt
		record.UpdatedAt = time.Now().UTC()
	}
	return mode, existingRecord, record, nil
}

func (h *Handler) registerAdminPlatformTemplateAITools(reg *aitool.Registry) {
	// 15. admin_list_platform_app_templates
	aitool.Register(reg, aitool.Spec[adminListPlatformAppTemplatesInput, map[string]any]{
		Name:              "admin_list_platform_app_templates",
		Domain:            aitool.DomainAdmin,
		Title:             "查看或导出平台应用模板",
		Summary:           "分页查看平台级应用市场模板目录（含下架项），或导出指定内置/平台模板为 luna.app-template/v1 Bundle",
		AliasesZH:         []string{"平台应用模板", "导出应用模板", "管理应用市场"},
		AliasesEN:         []string{"list platform app templates", "export app template"},
		AdminOnly:         true,
		GlobalSessionOnly: true,
		ApprovalTier:      aitool.ApprovalTierL0,
		Effect:            aitool.EffectRead,
		RequiredScopes:    []string{string(authz.ActionConfigRead)},
		Execute: func(tc *aitool.ToolContext, in adminListPlatformAppTemplatesInput) (map[string]any, error) {
			if _, err := h.requireAIPlatformAdmin(tc); err != nil {
				return nil, err
			}
			if in.Template != nil && strings.TrimSpace(*in.Template) != "" && in.ExportTemplate != nil && strings.TrimSpace(*in.ExportTemplate) != "" {
				return nil, aitool.BadRequest("argument_invalid", "specify either template (summary filter) or exportTemplate (full bundle export), not both")
			}

			// Bundle export mode
			if in.ExportTemplate != nil && strings.TrimSpace(*in.ExportTemplate) != "" {
				key := strings.TrimSpace(*in.ExportTemplate)
				if tc.DB != nil {
					rec, found, err := findPlatformTemplateRecord(tc.DB, key)
					if err != nil {
						return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
					}
					if found {
						tpl, err := appstore.TemplateFromPlatformModel(rec)
						if err != nil {
							return nil, aitool.NewError(http.StatusInternalServerError, "app_template.decode_failed", err.Error())
						}
						return map[string]any{
							"mode":       "bundle",
							"templateId": rec.ID,
							"slug":       rec.Slug,
							"source":     appstore.TemplateSourcePlatform,
							"bundle":     appstore.BundleFromTemplate(tpl, rec.Enabled),
						}, nil
					}
				}
				builtin, found, err := appstore.Find(key)
				if err != nil {
					return nil, aitool.NewError(http.StatusInternalServerError, "app_template.catalog_unavailable", err.Error())
				}
				if !found || isSystemComponentTemplate(builtin) {
					return nil, aitool.NotFound("app_template.not_found", "application template was not found")
				}
				return map[string]any{
					"mode":       "bundle",
					"templateId": builtin.ID,
					"slug":       builtin.Slug,
					"source":     appstore.TemplateSourceBuiltin,
					"bundle":     appstore.BundleFromTemplate(builtin, true),
				}, nil
			}

			// Summary list mode
			if in.Category != nil && strings.TrimSpace(*in.Category) != "" {
				cat := strings.TrimSpace(*in.Category)
				if !slices.Contains(appstore.AllowedCategories(), cat) {
					return nil, aitool.BadRequest("argument_invalid", fmt.Sprintf("invalid category %q; allowed values: %s", cat, strings.Join(appstore.AllowedCategories(), ", ")))
				}
			}
			page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
			if tc.DB == nil {
				return aitool.PageEnvelope([]aiPlatformAppTemplateSummaryItem{}, page, pageSize, "popularityWeight", "desc", 0), nil
			}

			query := tc.DB.Order("popularity_weight DESC, slug ASC")
			if in.Template != nil && strings.TrimSpace(*in.Template) != "" {
				key := strings.TrimSpace(*in.Template)
				query = query.Where("id = ? OR slug = ?", key, strings.ToLower(key))
			}
			if in.Category != nil && strings.TrimSpace(*in.Category) != "" {
				query = query.Where("LOWER(category) = ?", strings.ToLower(strings.TrimSpace(*in.Category)))
			}
			if in.Enabled != nil {
				query = query.Where("enabled = ?", *in.Enabled)
			}

			var records []model.PlatformAppTemplate
			if err := query.Find(&records).Error; err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}

			keyword := ""
			if in.Query != nil {
				keyword = strings.ToLower(strings.TrimSpace(*in.Query))
			}
			items := make([]aiPlatformAppTemplateSummaryItem, 0, len(records))
			for _, rec := range records {
				summary, tpl, err := platformTemplateSummaryItemFrom(rec)
				if err != nil {
					return nil, aitool.NewError(http.StatusInternalServerError, "app_template.decode_failed", err.Error())
				}
				if keyword != "" && !templateMatchesQuery(tpl, keyword) {
					continue
				}
				items = append(items, summary)
			}

			total := int64(len(items))
			start := (page - 1) * pageSize
			var paged []aiPlatformAppTemplateSummaryItem
			if start < len(items) {
				paged = items[start:min(start+pageSize, len(items))]
			} else {
				paged = []aiPlatformAppTemplateSummaryItem{}
			}
			return aitool.PageEnvelope(paged, page, pageSize, "popularityWeight", "desc", total), nil
		},
	})

	// 16. admin_save_platform_app_template
	aitool.Register(reg, aitool.Spec[adminSavePlatformAppTemplateInput, platformAppTemplateResponse]{
		Name:              "admin_save_platform_app_template",
		Domain:            aitool.DomainAdmin,
		Title:             "创建、导入或更新平台应用模板",
		Summary:           "导入 PlatformTemplateBundle 或创建/纯增量更新平台自定义应用市场模板",
		AliasesZH:         []string{"保存平台模板", "创建应用模板", "导入应用模板", "上架下架模板"},
		AliasesEN:         []string{"save platform app template", "import app template", "update app template"},
		AdminOnly:         true,
		GlobalSessionOnly: true,
		ApprovalTier:      aitool.ApprovalTierL2,
		Effect:            aitool.EffectWrite,
		RequiredScopes:    []string{string(authz.ActionConfigWrite)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "admin_list_platform_app_templates",
				Family:         "resource",
				IDFrom:         "result.id",
				ReadIDArgument: "template",
				Collection:     true,
				Expect:         "present",
			},
		},
		Preview: func(tc *aitool.ToolContext, in adminSavePlatformAppTemplateInput) (*aitool.PreviewResult, error) {
			user, err := h.requireAIPlatformAdmin(tc)
			if err != nil {
				return nil, err
			}
			mode, existing, record, err := h.prepareAIPlatformTemplateSave(tc, user, in)
			if err != nil {
				return nil, err
			}

			changes := make([]aitool.PreviewChange, 0, 6)
			targetID := record.Slug
			if existing != nil {
				targetID = existing.ID
				if existing.Slug != record.Slug {
					changes = append(changes, aitool.PreviewChange{Field: "slug", Before: existing.Slug, After: record.Slug, Action: "update"})
				}
				if existing.Name != record.Name {
					changes = append(changes, aitool.PreviewChange{Field: "name", Before: existing.Name, After: record.Name, Action: "update"})
				}
				if existing.Version != record.Version {
					changes = append(changes, aitool.PreviewChange{Field: "version", Before: existing.Version, After: record.Version, Action: "update"})
				}
				if existing.Enabled != record.Enabled {
					changes = append(changes, aitool.PreviewChange{Field: "enabled", Before: existing.Enabled, After: record.Enabled, Action: "update"})
				}
				if existing.Category != record.Category {
					changes = append(changes, aitool.PreviewChange{Field: "category", Before: existing.Category, After: record.Category, Action: "update"})
				}
			} else {
				changes = append(changes,
					aitool.PreviewChange{Field: "slug", After: record.Slug, Action: "create"},
					aitool.PreviewChange{Field: "name", After: record.Name, Action: "create"},
					aitool.PreviewChange{Field: "category", After: record.Category, Action: "create"},
					aitool.PreviewChange{Field: "enabled", After: record.Enabled, Action: "create"},
				)
			}

			existingID := ""
			if existing != nil {
				existingID = existing.ID
			}
			actionVerb := "Create"
			if existing != nil {
				actionVerb = "Update"
			}
			return &aitool.PreviewResult{
				Preview: aitool.ApprovalPreview{
					Summary:        fmt.Sprintf("%s platform app template %s (%s, enabled=%t)", actionVerb, record.Name, record.Slug, record.Enabled),
					TargetResource: "platform_app_template",
					TargetID:       targetID,
					Changes:        changes,
				},
				LockedPayload: platformTemplateLockedPayload{
					Mode:       mode,
					ExistingID: existingID,
					Slug:       record.Slug,
				},
			}, nil
		},
		Execute: func(tc *aitool.ToolContext, in adminSavePlatformAppTemplateInput) (platformAppTemplateResponse, error) {
			user, err := h.requireAIPlatformAdmin(tc)
			if err != nil {
				return platformAppTemplateResponse{}, err
			}
			mode, existing, record, err := h.prepareAIPlatformTemplateSave(tc, user, in)
			if err != nil {
				return platformAppTemplateResponse{}, err
			}
			if len(bytes.TrimSpace(tc.LockedPayload)) > 0 {
				var locked platformTemplateLockedPayload
				if err := tc.UnmarshalLockedPayload(&locked); err != nil {
					return platformAppTemplateResponse{}, aitool.Conflict("ai.preview_lock_invalid", err.Error())
				}
				existingID := ""
				if existing != nil {
					existingID = existing.ID
				}
				if locked.ExistingID != existingID {
					return platformAppTemplateResponse{}, aitool.Conflict("ai.preview_lock_mismatch", "platform template state changed since preview; please retry")
				}
			}

			if existing != nil {
				if err := tc.DB.Save(&record).Error; err != nil {
					return platformAppTemplateResponse{}, aitool.NewError(http.StatusInternalServerError, "database.write_failed", err.Error())
				}
			} else {
				if err := tc.DB.Create(&record).Error; err != nil {
					return platformAppTemplateResponse{}, aitool.NewError(http.StatusInternalServerError, "database.write_failed", err.Error())
				}
			}

			auditAction := "platform_app_template.create"
			auditMsg := fmt.Sprintf("slug=%s enabled=%t", record.Slug, record.Enabled)
			if mode == "import" {
				auditAction = "platform_app_template.import"
				auditMsg = fmt.Sprintf("slug=%s overwrite=%t enabled=%t", record.Slug, existing != nil, record.Enabled)
			} else if existing != nil {
				auditAction = "platform_app_template.update"
				if existing.Enabled != record.Enabled {
					if record.Enabled {
						auditAction = "platform_app_template.enable"
					} else {
						auditAction = "platform_app_template.disable"
					}
				}
			}
			h.auditWithContext(user.ID, auditAction, record.ID, true, auditMsg, tc.Context())

			resp, err := platformAppTemplateResponseFrom(record)
			if err != nil {
				return platformAppTemplateResponse{}, aitool.NewError(http.StatusInternalServerError, "app_template.decode_failed", err.Error())
			}
			return resp, nil
		},
	})

	// 17. admin_delete_platform_app_template
	aitool.Register(reg, aitool.Spec[adminDeletePlatformAppTemplateInput, map[string]any]{
		Name:              "admin_delete_platform_app_template",
		Domain:            aitool.DomainAdmin,
		Title:             "删除平台自定义应用模板",
		Summary:           "删除平台自定义应用市场模板（内置模板受保护不可删除）",
		AliasesZH:         []string{"删除平台模板", "移除应用模板"},
		AliasesEN:         []string{"delete platform app template", "remove platform template"},
		AdminOnly:         true,
		GlobalSessionOnly: true,
		ApprovalTier:      aitool.ApprovalTierL2,
		Effect:            aitool.EffectWrite,
		RequiredScopes:    []string{string(authz.ActionConfigWrite)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "admin_list_platform_app_templates",
				Family:         "resource",
				IDFrom:         "result.id",
				ReadIDArgument: "template",
				Collection:     true,
				Expect:         "absent",
			},
		},
		Preview: func(tc *aitool.ToolContext, in adminDeletePlatformAppTemplateInput) (*aitool.PreviewResult, error) {
			if _, err := h.requireAIPlatformAdmin(tc); err != nil {
				return nil, err
			}
			key := strings.TrimSpace(in.Template)
			if key == "" {
				return nil, aitool.BadRequest("argument_invalid", "template is required")
			}
			existing, found, err := findPlatformTemplateRecord(tc.DB, key)
			if err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}
			if !found {
				if isBuiltin, _ := appstore.IsBuiltinSlugOrID(key); isBuiltin {
					return nil, aitool.NewErrorWithHint(
						http.StatusConflict,
						"app_template.builtin_immutable",
						fmt.Sprintf("template %q is a built-in template and cannot be deleted", key),
						"Only custom platform templates stored in the database can be deleted.",
					)
				}
				return nil, aitool.NotFound("app_template.not_found", "platform app template was not found")
			}

			return &aitool.PreviewResult{
				Preview: aitool.ApprovalPreview{
					Summary:        fmt.Sprintf("Delete platform app template %s (%s)", existing.Name, existing.Slug),
					TargetResource: "platform_app_template",
					TargetID:       existing.ID,
					Changes: []aitool.PreviewChange{
						{Field: "slug", Before: existing.Slug, Action: "delete"},
					},
				},
				LockedPayload: platformTemplateLockedPayload{
					Mode:       "delete",
					ExistingID: existing.ID,
					Slug:       existing.Slug,
				},
			}, nil
		},
		Execute: func(tc *aitool.ToolContext, in adminDeletePlatformAppTemplateInput) (map[string]any, error) {
			user, err := h.requireAIPlatformAdmin(tc)
			if err != nil {
				return nil, err
			}
			var existing model.PlatformAppTemplate
			var found bool
			if len(bytes.TrimSpace(tc.LockedPayload)) > 0 {
				var locked platformTemplateLockedPayload
				if err := tc.UnmarshalLockedPayload(&locked); err != nil {
					return nil, aitool.Conflict("ai.preview_lock_invalid", err.Error())
				}
				existing, found, err = findPlatformTemplateRecord(tc.DB, locked.ExistingID)
			} else {
				existing, found, err = findPlatformTemplateRecord(tc.DB, in.Template)
			}
			if err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}
			if !found {
				return nil, aitool.NotFound("app_template.not_found", "platform app template was not found")
			}

			if err := tc.DB.Delete(&model.PlatformAppTemplate{}, "id = ?", existing.ID).Error; err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "database.write_failed", err.Error())
			}
			h.auditWithContext(user.ID, "platform_app_template.delete", existing.ID, true, fmt.Sprintf("slug=%s", existing.Slug), tc.Context())
			return map[string]any{
				"id":      existing.ID,
				"slug":    existing.Slug,
				"deleted": true,
			}, nil
		},
	})
}
