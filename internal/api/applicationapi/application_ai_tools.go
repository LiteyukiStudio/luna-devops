package applicationapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/resourceidentifier"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RegisterAITools registers the 17 domain facade AI tools owned by applicationapi:
//   - 8 delivery_* tools (applications & app marketplace templates)
//   - 6 network_* tools (topology & service bindings)
//   - 3 admin_* tools (platform app templates)
func (h *Handler) RegisterAITools(reg *aitool.Registry) {
	h.registerDeliveryApplicationAITools(reg)
	h.registerDeliveryTemplateAITools(reg)
	h.registerNetworkTopologyAndBindingAITools(reg)
	h.registerAdminPlatformTemplateAITools(reg)
}

func newAIToolGin(tc *aitool.ToolContext) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	subGin, _ := gin.CreateTestContext(rec)
	reqCtx := tc.Context()
	req, _ := http.NewRequestWithContext(reqCtx, http.MethodPost, "/internal/v1/ai/tools", http.NoBody)
	if tc != nil && tc.Gin != nil && tc.Gin.Request != nil {
		req.Header = tc.Gin.Request.Header.Clone()
		req.RemoteAddr = tc.Gin.Request.RemoteAddr
	}
	subGin.Request = req
	if tc != nil && tc.Gin != nil && tc.Gin.Keys != nil {
		subGin.Keys = maps.Clone(tc.Gin.Keys)
	}
	return subGin, rec
}

func aiToolErrorFromRecorder(rec *httptest.ResponseRecorder, fallbackCode, fallbackMessage string) *aitool.ToolError {
	status := http.StatusBadRequest
	if rec != nil && rec.Code >= 400 {
		status = rec.Code
	}
	if rec == nil {
		return aitool.NewError(status, fallbackCode, fallbackMessage)
	}
	raw := bytes.TrimSpace(rec.Body.Bytes())
	if len(raw) == 0 {
		return aitool.NewError(status, fallbackCode, fallbackMessage)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return aitool.NewError(status, fallbackCode, string(raw))
	}
	code, _ := parsed["code"].(string)
	if strings.TrimSpace(code) == "" {
		code = fallbackCode
	}
	msg, _ := parsed["message"].(string)
	if strings.TrimSpace(msg) == "" {
		msg = fallbackMessage
	}
	return &aitool.ToolError{
		Status:  status,
		Code:    code,
		Message: msg,
		RawBody: append(json.RawMessage(nil), raw...),
	}
}

func (h *Handler) authorizeAIProject(tc *aitool.ToolContext, projectRef string, action authz.Action) (model.User, model.Project, *gin.Context, *httptest.ResponseRecorder, error) {
	projectRef = strings.TrimSpace(projectRef)
	if projectRef == "" {
		return model.User{}, model.Project{}, nil, nil, aitool.BadRequest("project.required", "project is required")
	}
	c, rec := newAIToolGin(tc)
	user, project, ok := h.host.AuthorizeProjectRef(c, projectRef, action)
	if !ok {
		return model.User{}, model.Project{}, nil, nil, aiToolErrorFromRecorder(rec, "project.access_denied", "project authorization failed")
	}
	return user, project, c, rec, nil
}

func (h *Handler) ensureAIProjectMutable(tc *aitool.ToolContext, project model.Project) error {
	c, rec := newAIToolGin(tc)
	if !h.ensureProjectCanMutate(c, project) {
		return aiToolErrorFromRecorder(rec, "project.immutable", "project cannot be mutated")
	}
	return nil
}

func (h *Handler) ensureAIBillingAllowsDeploy(tc *aitool.ToolContext, projectID string) error {
	c, rec := newAIToolGin(tc)
	if !h.ensureBillingAllowsDeployChange(c, projectID) {
		return aiToolErrorFromRecorder(rec, "billing.insufficient_credits", "billing check blocked deployment mutation")
	}
	return nil
}

func (h *Handler) requireAIPlatformAdmin(tc *aitool.ToolContext) (model.User, error) {
	if tc != nil && strings.TrimSpace(tc.User.ID) != "" {
		if !authz.IsPlatformAdmin(tc.User.Role) {
			return model.User{}, aitool.Forbidden("config.admin.required", "platform administrator role is required")
		}
		return tc.User, nil
	}
	c, rec := newAIToolGin(tc)
	user, ok := h.requirePlatformTemplateAdmin(c)
	if !ok {
		return model.User{}, aiToolErrorFromRecorder(rec, "config.admin.required", "platform administrator role is required")
	}
	return user, nil
}

func applyAIKeywordSearch(query *gorm.DB, keyword string, columns ...string) *gorm.DB {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" || len(columns) == 0 {
		return query
	}
	escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(keyword)
	pattern := "%" + escaped + "%"
	conditions := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns))
	for _, col := range columns {
		conditions = append(conditions, "LOWER("+col+") LIKE LOWER(?) ESCAPE '\\'")
		args = append(args, pattern)
	}
	return query.Where(strings.Join(conditions, " OR "), args...)
}

func normalizeAISort(sortByRaw, sortOrderRaw *string, allowed map[string]string, defaultSortBy, defaultSortOrder string) (string, string, string, error) {
	sortBy := defaultSortBy
	if sortByRaw != nil && strings.TrimSpace(*sortByRaw) != "" {
		candidate := strings.TrimSpace(*sortByRaw)
		if _, ok := allowed[candidate]; !ok {
			keys := make([]string, 0, len(allowed))
			for k := range allowed {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			return "", "", "", aitool.BadRequest("argument_invalid", fmt.Sprintf("invalid sortBy %q; allowed values: %s", candidate, strings.Join(keys, ", ")))
		}
		sortBy = candidate
	}
	sortOrder := defaultSortOrder
	if sortOrderRaw != nil && strings.TrimSpace(*sortOrderRaw) != "" {
		candidate := strings.ToLower(strings.TrimSpace(*sortOrderRaw))
		if candidate != "asc" && candidate != "desc" {
			return "", "", "", aitool.BadRequest("argument_invalid", fmt.Sprintf("invalid sortOrder %q; allowed values: asc, desc", candidate))
		}
		sortOrder = candidate
	}
	return sortBy, sortOrder, allowed[sortBy] + " " + sortOrder, nil
}

func validateAIApplicationIcon(raw *string, defaultOnEmpty bool) (string, bool, error) {
	if raw == nil {
		if defaultOnEmpty {
			return "box", true, nil
		}
		return "", false, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		if defaultOnEmpty {
			return "box", true, nil
		}
		return "", false, nil
	}
	if !isApplicationIconReference(trimmed) {
		return "", false, aitool.NewErrorWithHint(
			http.StatusBadRequest,
			"argument_invalid",
			fmt.Sprintf("invalid application icon %q", trimmed),
			"Use one of the built-in icon names ("+strings.Join(applicationIconNames, ", ")+"), a site-relative /path, or an http(s):// URL.",
		)
	}
	return trimmed, true, nil
}

// --- DTOs & Input Structs for Application Tools ---

type deliveryListApplicationsInput struct {
	Project        string  `json:"project" desc:"项目 ID 或 identifier"`
	Search         *string `json:"search,omitempty" desc:"按名称或标识模糊搜索"`
	IncludeRuntime *bool   `json:"includeRuntime,omitempty" default:"true" desc:"是否返回各环境实时健康度、observationCode 与主访问 URL"`
	SortBy         *string `json:"sortBy,omitempty" enum:"createdAt|name|identifier" default:"createdAt" desc:"排序字段"`
	SortOrder      *string `json:"sortOrder,omitempty" enum:"asc|desc" default:"desc" desc:"排序方向"`
	Page           *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码"`
	PageSize       *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（上限 50）"`
}

type deliveryGetApplicationInput struct {
	Project     string `json:"project" desc:"项目 ID 或 identifier"`
	Application string `json:"application" desc:"应用 ID、唯一 identifier 或唯一名称"`
}

type aiApplicationTargetDetail struct {
	ID                 string                     `json:"id"`
	Name               string                     `json:"name"`
	Stage              string                     `json:"stage"`
	ClusterID          string                     `json:"clusterId"`
	EffectiveClusterID string                     `json:"effectiveClusterId"`
	ClusterName        string                     `json:"clusterName"`
	WorkloadType       string                     `json:"workloadType"`
	SourceType         string                     `json:"sourceType"`
	Enabled            bool                       `json:"enabled"`
	RequireApproval    bool                       `json:"requireApproval"`
	DeleteStatus       string                     `json:"deleteStatus"`
	DeleteCode         string                     `json:"deleteCode,omitempty"`
	Deployed           bool                       `json:"deployed"`
	DesiredReplicas    int32                      `json:"desiredReplicas"`
	ReadyReplicas      int32                      `json:"readyReplicas"`
	Status             string                     `json:"status"`
	ObservationCode    string                     `json:"observationCode,omitempty"`
	CurrentRelease     *applicationCurrentRelease `json:"currentRelease,omitempty"`
}

type aiApplicationBoundVolume struct {
	BindingID            string `json:"bindingId"`
	DeploymentTargetID   string `json:"deploymentTargetId"`
	DeploymentTargetName string `json:"deploymentTargetName"`
	Stage                string `json:"stage"`
	ProjectVolumeID      string `json:"projectVolumeId"`
	DisplayName          string `json:"displayName"`
	LogicalName          string `json:"logicalName"`
	MountPath            string `json:"mountPath,omitempty"`
	DevicePath           string `json:"devicePath,omitempty"`
	ActivationState      string `json:"activationState"`
}

type deliveryCreateApplicationInput struct {
	Project    string  `json:"project" desc:"项目 ID 或 identifier"`
	Identifier string  `json:"identifier" desc:"应用唯一英文标识（DNS/slug 规范，创建后不可修改）"`
	Name       string  `json:"name" desc:"应用显示名称"`
	Icon       *string `json:"icon,omitempty" desc:"内置 32 种图标枚举名（如 box、server、database、globe）或合法 http(s):// / 站内图标路径"`
}

type deliveryUpdateApplicationInput struct {
	Project     string  `json:"project" desc:"项目 ID 或 identifier"`
	Application string  `json:"application" desc:"应用 ID、唯一 identifier 或唯一名称"`
	Name        *string `json:"name,omitempty" desc:"新应用显示名称"`
	Icon        *string `json:"icon,omitempty" desc:"新图标（传空串不修改；重置默认图标请传 resetIcon=true）"`
	ResetIcon   *bool   `json:"resetIcon,omitempty" desc:"是否重置为默认图标 box"`
}

type deliveryDeleteApplicationInput struct {
	Project     string `json:"project" desc:"项目 ID 或 identifier"`
	Application string `json:"application" desc:"应用 ID、唯一 identifier 或唯一名称"`
}

type deleteApplicationLockedPayload struct {
	ProjectID     string `json:"projectId"`
	ApplicationID string `json:"applicationId"`
}

func aiApplicationMutationPayload(app model.Application) map[string]any {
	return map[string]any{
		"id":               app.ID,
		"applicationId":    app.ID,
		"projectId":        app.ProjectID,
		"identifier":       app.Identifier,
		"name":             app.Name,
		"icon":             app.Icon,
		"deleteStatus":     app.DeleteStatus,
		"deleteMessage":    app.DeleteMessage,
		"deleteCode":       app.DeleteCode,
		"deleteStartedAt":  app.DeleteStartedAt,
		"deleteFinishedAt": app.DeleteFinishedAt,
		"createdAt":        app.CreatedAt,
		"updatedAt":        app.UpdatedAt,
	}
}

func (h *Handler) registerDeliveryApplicationAITools(reg *aitool.Registry) {
	// 1. delivery_list_applications
	aitool.Register(reg, aitool.Spec[deliveryListApplicationsInput, map[string]any]{
		Name:           "delivery_list_applications",
		Domain:         aitool.DomainDelivery,
		Title:          "分页列出项目应用",
		Summary:        "分页列出项目内应用及各环境运行状态摘要",
		AliasesZH:      []string{"列出应用", "应用列表", "查看项目应用"},
		AliasesEN:      []string{"list applications", "list apps"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionApplicationRead)},
		Execute: func(tc *aitool.ToolContext, in deliveryListApplicationsInput) (map[string]any, error) {
			_, project, _, _, err := h.authorizeAIProject(tc, in.Project, authz.ActionApplicationRead)
			if err != nil {
				return nil, err
			}
			page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
			sortBy, sortOrder, orderClause, err := normalizeAISort(in.SortBy, in.SortOrder, map[string]string{
				"createdAt":  "created_at",
				"name":       "name",
				"identifier": "identifier",
			}, "createdAt", "desc")
			if err != nil {
				return nil, err
			}

			db := tc.DB
			query := db.Model(&model.Application{}).Where("project_id = ?", project.ID)
			if in.Search != nil {
				query = applyAIKeywordSearch(query, *in.Search, "name", "identifier")
			}
			var total int64
			if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}
			var applications []model.Application
			offset := (page - 1) * pageSize
			if err := query.Order(orderClause).Limit(pageSize).Offset(offset).Find(&applications).Error; err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}

			includeRuntime := in.IncludeRuntime == nil || *in.IncludeRuntime
			if includeRuntime {
				items, err := h.applicationListItemsWithRuntime(tc.Context(), project, applications)
				if err != nil {
					return nil, aitool.NewError(http.StatusInternalServerError, "application.runtime_summary_failed", err.Error())
				}
				tc.SetNoStore()
				out := aitool.PageEnvelope(items, page, pageSize, sortBy, sortOrder, total)
				out["projectId"] = project.ID
				return out, nil
			}

			out := aitool.PageEnvelope(applications, page, pageSize, sortBy, sortOrder, total)
			out["projectId"] = project.ID
			return out, nil
		},
	})

	// 2. delivery_get_application
	aitool.Register(reg, aitool.Spec[deliveryGetApplicationInput, map[string]any]{
		Name:           "delivery_get_application",
		Domain:         aitool.DomainDelivery,
		Title:          "查看单个应用详情",
		Summary:        "查看单个应用详情、下属部署目标清单摘要、服务依赖计数与绑定数据卷概览",
		AliasesZH:      []string{"应用详情", "获取应用", "查看应用环境"},
		AliasesEN:      []string{"get application", "inspect application"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionApplicationRead)},
		Execute: func(tc *aitool.ToolContext, in deliveryGetApplicationInput) (map[string]any, error) {
			_, project, _, _, err := h.authorizeAIProject(tc, in.Project, authz.ActionApplicationRead)
			if err != nil {
				return nil, err
			}
			db := tc.DB
			app, err := aitool.ResolveApplication(db, project.ID, in.Application)
			if err != nil {
				return nil, err
			}
			tc.SetNoStore()

			var targets []model.DeploymentTarget
			if err := db.Where("project_id = ? AND application_id = ? AND delete_status <> ?", project.ID, app.ID, "deleted").
				Order("created_at asc").
				Find(&targets).Error; err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}
			h.host.ObserveDeploymentTargets(tc.Context(), project, targets)

			accessURLs, err := h.applicationPrimaryAccessURLs(tc.Context(), project, []string{app.ID}, targets)
			if err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "application.access_url_failed", err.Error())
			}
			targetReleases, err := h.deploymentTargetCurrentReleases(tc.Context(), project, targets)
			if err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "application.releases_failed", err.Error())
			}
			deploymentSummary := summarizeApplicationDeploymentTargets(targets, targetReleases)
			currentRelease := applicationCurrentReleaseOf(targets, targetReleases)

			// Resolve effective cluster IDs and cluster names
			defaultClusterID := ""
			clusterIDSet := make(map[string]struct{}, len(targets))
			for idx := range targets {
				effID := firstNonEmpty(targets[idx].EffectiveClusterID, targets[idx].ClusterID)
				if effID == "" {
					if defaultClusterID == "" {
						defaultClusterID = h.defaultRuntimeClusterID(tc.Context())
					}
					effID = defaultClusterID
				}
				targets[idx].EffectiveClusterID = effID
				if effID != "" {
					clusterIDSet[effID] = struct{}{}
				}
				if cid := strings.TrimSpace(targets[idx].ClusterID); cid != "" {
					clusterIDSet[cid] = struct{}{}
				}
			}
			clusterNames := make(map[string]string, len(clusterIDSet))
			if len(clusterIDSet) > 0 {
				ids := make([]string, 0, len(clusterIDSet))
				for cid := range clusterIDSet {
					ids = append(ids, cid)
				}
				var clusters []model.RuntimeCluster
				if err := db.Select("id", "name").Where("id IN ?", ids).Find(&clusters).Error; err == nil {
					for _, cl := range clusters {
						clusterNames[cl.ID] = cl.Name
					}
				}
			}

			stageByTargetID := make(map[string]string, len(targets))
			targetDetails := make([]aiApplicationTargetDetail, 0, len(targets))
			for _, target := range targets {
				stageByTargetID[target.ID] = target.Stage
				rel := targetReleases[target.ID]
				deployed := rel != nil
				workloadType := strings.TrimSpace(target.WorkloadType)
				if workloadType == "" {
					workloadType = "Deployment"
				}
				sourceType := strings.TrimSpace(target.SourceType)
				if sourceType == "" {
					sourceType = "repository"
				}
				targetDetails = append(targetDetails, aiApplicationTargetDetail{
					ID:                 target.ID,
					Name:               target.Name,
					Stage:              target.Stage,
					ClusterID:          target.ClusterID,
					EffectiveClusterID: target.EffectiveClusterID,
					ClusterName:        clusterNames[target.EffectiveClusterID],
					WorkloadType:       workloadType,
					SourceType:         sourceType,
					Enabled:            target.Enabled,
					RequireApproval:    target.RequireApproval,
					DeleteStatus:       target.DeleteStatus,
					DeleteCode:         target.DeleteCode,
					Deployed:           deployed,
					DesiredReplicas:    target.DesiredReplicas,
					ReadyReplicas:      target.ReadyReplicas,
					Status:             applicationTargetRuntimeStatus(target, deployed),
					ObservationCode:    target.ObservationCode,
					CurrentRelease:     rel,
				})
			}

			var inboundCount int64
			var outboundCount int64
			_ = db.Model(&model.ServiceBinding{}).
				Where("project_id = ? AND target_application_id = ? AND source_application_id <> ? AND enabled = ?", project.ID, app.ID, app.ID, true).
				Count(&inboundCount).Error
			_ = db.Model(&model.ServiceBinding{}).
				Where("project_id = ? AND source_application_id = ? AND target_application_id <> ? AND enabled = ?", project.ID, app.ID, app.ID, true).
				Count(&outboundCount).Error

			delPreview, err := h.buildApplicationDeletionPreview(tc.Context(), project, app)
			if err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "application.volume_preview_failed", err.Error())
			}
			boundVolumes := make([]aiApplicationBoundVolume, 0)
			for _, tp := range delPreview.Targets {
				for _, vp := range tp.Volumes {
					boundVolumes = append(boundVolumes, aiApplicationBoundVolume{
						BindingID:            vp.BindingID,
						DeploymentTargetID:   tp.TargetID,
						DeploymentTargetName: tp.TargetName,
						Stage:                stageByTargetID[tp.TargetID],
						ProjectVolumeID:      vp.ProjectVolumeID,
						DisplayName:          vp.DisplayName,
						LogicalName:          vp.LogicalName,
						MountPath:            vp.MountPath,
						DevicePath:           vp.DevicePath,
						ActivationState:      vp.ActivationState,
					})
				}
			}

			out := aiApplicationMutationPayload(app)
			out["primaryAccessUrl"] = accessURLs[app.ID]
			out["currentRelease"] = currentRelease
			out["deploymentSummary"] = deploymentSummary
			out["targets"] = targetDetails
			out["inboundServiceBindingCount"] = inboundCount
			out["outboundServiceBindingCount"] = outboundCount
			out["boundVolumes"] = boundVolumes
			out["hasPersistentData"] = delPreview.HasPersistentData
			return out, nil
		},
	})

	// 5. delivery_create_application
	aitool.Register(reg, aitool.Spec[deliveryCreateApplicationInput, map[string]any]{
		Name:           "delivery_create_application",
		Domain:         aitool.DomainDelivery,
		Title:          "创建新应用",
		Summary:        "在指定项目空间下创建新应用",
		AliasesZH:      []string{"新建应用", "创建应用"},
		AliasesEN:      []string{"create application", "new app"},
		ApprovalTier:   aitool.ApprovalTierL1,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionApplicationCreate)},
		Verification: []aitool.OperationVerification{
			{OperationID: "delivery_get_application", Family: "application", Expect: "present"},
		},
		Execute: func(tc *aitool.ToolContext, in deliveryCreateApplicationInput) (map[string]any, error) {
			user, project, _, _, err := h.authorizeAIProject(tc, in.Project, authz.ActionApplicationCreate)
			if err != nil {
				return nil, err
			}
			if err := h.ensureAIProjectMutable(tc, project); err != nil {
				return nil, err
			}

			identifier := strings.TrimSpace(in.Identifier)
			if err := resourceidentifier.Validate(identifier, applicationIdentifierMinLength, applicationIdentifierMaxLength); err != nil {
				return nil, aitool.BadRequest("application.identifier_invalid", err.Error())
			}
			name := strings.TrimSpace(in.Name)
			if name == "" {
				return nil, aitool.BadRequest("argument_invalid", "application name is required")
			}
			icon, _, err := validateAIApplicationIcon(in.Icon, true)
			if err != nil {
				return nil, err
			}

			c, rec := newAIToolGin(tc)
			if !h.ensureApplicationIdentifierAvailable(c, project.ID, identifier, "") {
				return nil, aiToolErrorFromRecorder(rec, "application.identifier_exists", "application identifier already exists")
			}

			app := model.Application{
				ID:           id.New("app"),
				ProjectID:    project.ID,
				Identifier:   identifier,
				Name:         name,
				Icon:         icon,
				DeleteStatus: "active",
			}
			if err := createApplicationRecord(tc.DB, &app); errors.Is(err, errApplicationIdentifierExists) {
				return nil, aitool.Conflict("application.identifier_exists", "application identifier already exists in this project")
			} else if err != nil {
				return nil, aitool.BadRequest("application.create_failed", err.Error())
			}

			h.auditWithContext(user.ID, "application.create", app.ID, true, app.Identifier, tc.Context())
			return aiApplicationMutationPayload(app), nil
		},
	})

	// 6. delivery_update_application
	aitool.Register(reg, aitool.Spec[deliveryUpdateApplicationInput, map[string]any]{
		Name:           "delivery_update_application",
		Domain:         aitool.DomainDelivery,
		Title:          "修改应用名称或图标",
		Summary:        "局部增量修改应用显示名称或图标",
		AliasesZH:      []string{"更新应用", "修改应用名称", "修改应用图标"},
		AliasesEN:      []string{"update application", "rename application"},
		ApprovalTier:   aitool.ApprovalTierL1,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionApplicationUpdate)},
		Verification: []aitool.OperationVerification{
			{OperationID: "delivery_get_application", Family: "application", Expect: "present"},
		},
		Execute: func(tc *aitool.ToolContext, in deliveryUpdateApplicationInput) (map[string]any, error) {
			user, project, _, _, err := h.authorizeAIProject(tc, in.Project, authz.ActionApplicationUpdate)
			if err != nil {
				return nil, err
			}
			if err := h.ensureAIProjectMutable(tc, project); err != nil {
				return nil, err
			}
			app, err := aitool.ResolveApplication(tc.DB, project.ID, in.Application)
			if err != nil {
				return nil, err
			}
			if !applicationCanMutate(app) {
				return nil, aitool.Conflict("application.delete_in_progress", "application is currently being deleted and cannot be modified")
			}

			resetIcon := in.ResetIcon != nil && *in.ResetIcon
			if resetIcon && in.Icon != nil && strings.TrimSpace(*in.Icon) != "" {
				return nil, aitool.BadRequest("argument_invalid", "cannot specify both icon and resetIcon=true")
			}

			iconVal, hasIconChange, err := validateAIApplicationIcon(in.Icon, false)
			if err != nil {
				return nil, err
			}
			hasNameChange := in.Name != nil
			if !hasNameChange && !hasIconChange && !resetIcon {
				return nil, aitool.BadRequest("argument_invalid", "at least one of name, icon, or resetIcon=true must be provided")
			}

			if hasNameChange {
				newName := strings.TrimSpace(*in.Name)
				if newName == "" {
					return nil, aitool.BadRequest("argument_invalid", "application name cannot be empty")
				}
				app.Name = newName
			}
			if resetIcon {
				app.Icon = "box"
			} else if hasIconChange {
				app.Icon = iconVal
			}

			if err := tc.DB.Save(&app).Error; err != nil {
				return nil, aitool.BadRequest("application.update_failed", err.Error())
			}
			h.auditWithContext(user.ID, "application.update", app.ID, true, app.Identifier, tc.Context())
			return aiApplicationMutationPayload(app), nil
		},
	})

	// 7. delivery_delete_application
	aitool.Register(reg, aitool.Spec[deliveryDeleteApplicationInput, map[string]any]{
		Name:           "delivery_delete_application",
		Domain:         aitool.DomainDelivery,
		Title:          "异步删除应用",
		Summary:        "异步删除应用及其下属部署环境，自动解绑并保留持久化数据卷",
		AliasesZH:      []string{"删除应用", "移除应用"},
		AliasesEN:      []string{"delete application", "remove app"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectAsync,
		RequiredScopes: []string{string(authz.ActionApplicationDelete)},
		Verification: []aitool.OperationVerification{
			{OperationID: "delivery_get_application", Family: "application", Expect: "absent"},
		},
		Preview: func(tc *aitool.ToolContext, in deliveryDeleteApplicationInput) (*aitool.PreviewResult, error) {
			_, project, _, _, err := h.authorizeAIProject(tc, in.Project, authz.ActionApplicationDelete)
			if err != nil {
				return nil, err
			}
			if err := h.ensureAIProjectMutable(tc, project); err != nil {
				return nil, err
			}
			app, err := aitool.ResolveApplication(tc.DB, project.ID, in.Application)
			if err != nil {
				return nil, err
			}
			if !applicationCanMutate(app) {
				return nil, aitool.Conflict("application.delete_in_progress", "application is already being deleted")
			}

			usages, err := incomingServiceBindingUsages(tc.DB, project.ID, app.ID, "")
			if err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}
			if len(usages) > 0 {
				conflictErr := aitool.NewErrorWithHint(
					http.StatusConflict,
					"service_binding_in_use",
					fmt.Sprintf("application %q (%s) is referenced by %d active inbound service binding(s)", app.Name, app.Identifier, len(usages)),
					"Delete or redirect the inbound service bindings first via network_delete_service_binding or network_save_service_binding.",
				)
				conflictErr.Details = map[string]any{"affectedSources": usages}
				return nil, conflictErr
			}

			delPreview, err := h.buildApplicationDeletionPreview(tc.Context(), project, app)
			if err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "application.delete_preview_failed", err.Error())
			}
			var routes []model.GatewayRoute
			_ = tc.DB.Select("id", "host", "path").
				Where("project_id = ? AND application_id = ? AND delete_status <> ?", project.ID, app.ID, "deleted").
				Find(&routes).Error

			impact := []string{
				fmt.Sprintf("Will delete %d deployment target(s) under application %s (%s)", len(delPreview.Targets), app.Name, app.Identifier),
			}
			var volumeNames []string
			for _, tp := range delPreview.Targets {
				for _, vp := range tp.Volumes {
					volumeNames = append(volumeNames, fmt.Sprintf("%s (%s)", vp.DisplayName, vp.ProjectVolumeID))
				}
			}
			slices.Sort(volumeNames)
			volumeNames = slices.Compact(volumeNames)
			if len(volumeNames) > 0 {
				impact = append(impact, fmt.Sprintf("Will unbind and retain %d ProjectVolume(s): %s", len(volumeNames), strings.Join(volumeNames, ", ")))
			}
			if len(routes) > 0 {
				hosts := make([]string, 0, len(routes))
				for _, r := range routes {
					hosts = append(hosts, r.Host+r.Path)
				}
				impact = append(impact, fmt.Sprintf("Will cascade-delete %d GatewayRoute(s): %s", len(routes), strings.Join(hosts, ", ")))
			}

			return &aitool.PreviewResult{
				Preview: aitool.ApprovalPreview{
					Summary:        fmt.Sprintf("Delete application %s (%s) and %d deployment target(s)", app.Name, app.Identifier, len(delPreview.Targets)),
					TargetResource: "application",
					TargetID:       app.ID,
					ProjectID:      project.ID,
					Changes: []aitool.PreviewChange{
						{Field: "deleteStatus", Before: app.DeleteStatus, After: "deleting", Action: "delete"},
					},
					Impact: impact,
				},
				LockedPayload: deleteApplicationLockedPayload{
					ProjectID:     project.ID,
					ApplicationID: app.ID,
				},
			}, nil
		},
		Execute: func(tc *aitool.ToolContext, in deliveryDeleteApplicationInput) (map[string]any, error) {
			user, project, c, rec, err := h.authorizeAIProject(tc, in.Project, authz.ActionApplicationDelete)
			if err != nil {
				return nil, err
			}
			if err := h.ensureAIProjectMutable(tc, project); err != nil {
				return nil, err
			}

			var app model.Application
			if len(bytes.TrimSpace(tc.LockedPayload)) > 0 {
				var locked deleteApplicationLockedPayload
				if err := tc.UnmarshalLockedPayload(&locked); err != nil {
					return nil, aitool.Conflict("ai.preview_lock_invalid", err.Error())
				}
				if locked.ProjectID != project.ID || strings.TrimSpace(locked.ApplicationID) == "" {
					return nil, aitool.Conflict("ai.preview_lock_mismatch", "preview lock does not match target project or application")
				}
				if err := tc.DB.First(&app, "id = ? AND project_id = ?", locked.ApplicationID, project.ID).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return nil, aitool.NotFound("application.not_found", "application was not found")
					}
					return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
				}
			} else {
				app, err = aitool.ResolveApplication(tc.DB, project.ID, in.Application)
				if err != nil {
					return nil, err
				}
			}

			if !applicationCanMutate(app) {
				return nil, aitool.Conflict("application.delete_in_progress", "application is already being deleted")
			}

			var targets []model.DeploymentTarget
			if err := tc.DB.Where("project_id = ? AND application_id = ?", app.ProjectID, app.ID).Find(&targets).Error; err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
			}

			startedAt := time.Now()
			volumeChanges := make(map[string]deploymentVolumeMountChanges, len(targets))
			txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
				var locked model.Application
				if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
					First(&locked, "id = ? AND project_id = ?", app.ID, app.ProjectID).Error; err != nil {
					return err
				}
				if !applicationCanMutate(locked) {
					return errApplicationDeleteInProgress
				}
				if err := ensureNoIncomingServiceBindingsTx(tx, app.ProjectID, app.ID, ""); err != nil {
					return err
				}
				if err := tx.Model(&model.Application{}).
					Where("id = ? AND project_id = ?", app.ID, app.ProjectID).
					Updates(map[string]any{
						"delete_status":      "deleting",
						"delete_code":        "",
						"delete_message":     "",
						"delete_started_at":  &startedAt,
						"delete_finished_at": nil,
					}).Error; err != nil {
					return err
				}
				app.DeleteStatus = "deleting"
				app.DeleteCode = ""
				app.DeleteMessage = ""
				app.DeleteStartedAt = &startedAt
				app.DeleteFinishedAt = nil
				for _, target := range targets {
					changes, syncErr := h.host.SyncDeploymentTargetVolumeMounts(tc.Context(), tx, target, nil)
					volumeChanges[target.ID] = changes
					if syncErr != nil {
						return syncErr
					}
				}
				return nil
			})

			var inUse *serviceBindingInUseError
			switch {
			case errors.As(txErr, &inUse):
				conflictErr := aitool.NewErrorWithHint(
					http.StatusConflict,
					"service_binding_in_use",
					"application is referenced by active inbound service bindings",
					"Delete or redirect the inbound service bindings first.",
				)
				conflictErr.Details = map[string]any{"affectedSources": inUse.usages}
				return nil, conflictErr
			case errors.Is(txErr, errApplicationDeleteInProgress):
				return nil, aitool.Conflict("application.delete_in_progress", "application is already being deleted")
			case txErr != nil:
				for _, target := range targets {
					h.auditDeploymentVolumeMountFailure(tc.Context(), user.ID, volumeChanges[target.ID], txErr)
				}
				if volume.ErrorCode(txErr) != "" {
					h.host.WriteVolumeError(c, txErr)
					return nil, aiToolErrorFromRecorder(rec, volume.ErrorCode(txErr), txErr.Error())
				}
				return nil, aitool.NewError(http.StatusInternalServerError, "application.delete_failed", txErr.Error())
			}

			for _, target := range targets {
				h.auditDeploymentVolumeMountChanges(tc.Context(), user.ID, target, volumeChanges[target.ID])
			}
			if !h.enqueueApplicationDelete(tc.Context(), app, user.ID, false) {
				finishedAt := time.Now()
				_ = tc.DB.Model(&model.Application{}).Where("id = ?", app.ID).Updates(map[string]any{
					"delete_status":      "delete_failed",
					"delete_code":        applicationDeleteEnqueueFailedCode,
					"delete_message":     "application delete task enqueue failed",
					"delete_finished_at": &finishedAt,
				}).Error
				return nil, aitool.NewError(http.StatusServiceUnavailable, applicationDeleteEnqueueFailedCode, "application delete task enqueue failed")
			}

			h.auditWithContext(user.ID, "application.delete.request", app.ID, true, "volume_mounts_unbound", tc.Context())
			return map[string]any{
				"id":            app.ID,
				"applicationId": app.ID,
				"projectId":     project.ID,
				"deleteStatus":  "deleting",
			}, nil
		},
	})
}
