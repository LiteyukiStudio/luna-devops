package projectapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

const (
	maxHookScriptBytes         = 64 * 1024
	maxHookScriptPreviewRunes  = 512
	maxHookRunSnapshotBytes    = 32768
	defaultHookRunLogTailBytes = 8192
	maxHookRunLogTailBytes     = 32768
)

type runtimeListHooksInput struct {
	Project   string  `json:"project" desc:"项目 ID 或 identifier"`
	Hook      *string `json:"hook,omitempty" desc:"Hook ID（hook_xxx）或同项目唯一名称；传入时返回完整 script 正文"`
	SortBy    *string `json:"sortBy,omitempty" enum:"createdAt|name" default:"createdAt" desc:"排序字段"`
	SortOrder *string `json:"sortOrder,omitempty" enum:"asc|desc" default:"desc" desc:"排序方向"`
	Page      *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码"`
	PageSize  *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（上限 50）"`
}

type runtimeHookSummaryItem struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"projectId"`
	Name            string    `json:"name"`
	ScriptBytes     int       `json:"scriptBytes"`
	ScriptLines     int       `json:"scriptLines"`
	ScriptPreview   string    `json:"scriptPreview"`
	ScriptTruncated bool      `json:"scriptTruncated"`
	Shell           string    `json:"shell"`
	TimeoutSeconds  int       `json:"timeoutSeconds"`
	FailurePolicy   string    `json:"failurePolicy"`
	CreatedBy       string    `json:"createdBy"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type runtimeHookDetailResponse struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"projectId"`
	Name           string    `json:"name"`
	Script         string    `json:"script"`
	ScriptBytes    int       `json:"scriptBytes"`
	ScriptLines    int       `json:"scriptLines"`
	Shell          string    `json:"shell"`
	TimeoutSeconds int       `json:"timeoutSeconds"`
	FailurePolicy  string    `json:"failurePolicy"`
	CreatedBy      string    `json:"createdBy"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type runtimeListHookRunsInput struct {
	Project      string  `json:"project" desc:"项目 ID 或 identifier"`
	BuildRunID   *string `json:"buildRunId,omitempty" desc:"按构建记录 ID 过滤"`
	ReleaseID    *string `json:"releaseId,omitempty" desc:"按发布记录 ID 过滤"`
	Phase        *string `json:"phase,omitempty" enum:"prePull|postPull|preBuild|postBuild|prePush|postPush|preDeployment|postDeployment" desc:"按生命周期阶段过滤"`
	Status       *string `json:"status,omitempty" enum:"queued|running|succeeded|failed|canceled" desc:"按执行状态过滤"`
	HookRunID    *string `json:"hookRunId,omitempty" desc:"精确查看单条执行记录、截断脚本快照与尾部日志"`
	LogTailBytes *int    `json:"logTailBytes,omitempty" min:"1" max:"32768" default:"8192" desc:"单条详情模式下返回的日志尾部字节上限"`
	Page         *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码"`
	PageSize     *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（上限 50）"`
}

type runtimeHookRunListItem struct {
	ID                  string     `json:"id"`
	ProjectID           string     `json:"projectId"`
	HookConfigID        string     `json:"hookConfigId"`
	BuildRunID          string     `json:"buildRunId,omitempty"`
	BuildJobID          string     `json:"buildJobId,omitempty"`
	ReleaseID           string     `json:"releaseId,omitempty"`
	ApplicationID       string     `json:"applicationId,omitempty"`
	DeploymentTargetID  string     `json:"deploymentTargetId,omitempty"`
	Name                string     `json:"name"`
	Phase               string     `json:"phase"`
	Status              string     `json:"status"`
	ScriptSnapshotBytes int        `json:"scriptSnapshotBytes"`
	Shell               string     `json:"shell"`
	ImageRef            string     `json:"imageRef,omitempty"`
	TimeoutSeconds      int        `json:"timeoutSeconds"`
	FailurePolicy       string     `json:"failurePolicy"`
	ExitCode            int        `json:"exitCode"`
	Message             string     `json:"message,omitempty"`
	StartedAt           *time.Time `json:"startedAt,omitempty"`
	FinishedAt          *time.Time `json:"finishedAt,omitempty"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}

type runtimeSaveHookInput struct {
	Project        string  `json:"project" desc:"项目 ID 或 identifier"`
	Hook           *string `json:"hook,omitempty" desc:"省略为新建；传入 Hook ID（hook_xxx）或同项目唯一名称为纯增量更新"`
	Name           *string `json:"name,omitempty" desc:"脚本名称（新建必填）"`
	Script         *string `json:"script,omitempty" desc:"脚本内容（新建必填，<= 65536 字节；更新时省略保留原脚本）"`
	Shell          *string `json:"shell,omitempty" enum:"sh|bash" desc:"执行 Shell（新建默认 sh；更新时省略保留原值）"`
	TimeoutSeconds *int    `json:"timeoutSeconds,omitempty" min:"1" max:"3600" desc:"超时时间秒数（1..3600，新建默认 300）"`
	FailurePolicy  *string `json:"failurePolicy,omitempty" enum:"fail|ignore" desc:"失败策略（新建默认 fail；更新时省略保留原值）"`
}

type runtimeDeleteHookInput struct {
	Project string `json:"project" desc:"项目 ID 或 identifier"`
	Hook    string `json:"hook" desc:"Hook ID（hook_xxx）或同项目唯一名称"`
}

type lockedHookMutation struct {
	ProjectID      string `json:"projectId"`
	ResolvedHookID string `json:"resolvedHookId,omitempty"`
}

func (h *Handler) registerRuntimeHookAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[runtimeListHooksInput, map[string]any]{
		Name:           "runtime_list_hooks",
		Domain:         aitool.DomainRuntime,
		Title:          "查看项目生命周期脚本",
		Summary:        "分页列出项目生命周期脚本配置（ProjectHookConfig），或查看单个脚本完整内容",
		Tags:           []string{"hooks", "lifecycle"},
		AliasesZH:      []string{"生命周期脚本列表", "查看钩子脚本", "查看 Hook"},
		AliasesEN:      []string{"list hooks", "get hook script", "lifecycle hooks"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionProjectRead)},
		Execute:        h.aiRuntimeListHooks,
	})

	aitool.Register(reg, aitool.Spec[runtimeListHookRunsInput, map[string]any]{
		Name:           "runtime_list_hook_runs",
		Domain:         aitool.DomainRuntime,
		Title:          "查看生命周期脚本执行记录",
		Summary:        "分页查询项目构建或发布触发的生命周期脚本执行记录，支持按需读取单条脚本快照与截断日志",
		Tags:           []string{"hooks", "logs", "lifecycle"},
		AliasesZH:      []string{"查看钩子执行记录", "查看 Hook 日志", "生命周期钩子运行历史"},
		AliasesEN:      []string{"list hook runs", "get hook run log", "hook execution history"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionProjectRead)},
		Execute:        h.aiRuntimeListHookRuns,
	})

	aitool.Register(reg, aitool.Spec[runtimeSaveHookInput, runtimeHookDetailResponse]{
		Name:           "runtime_save_hook",
		Domain:         aitool.DomainRuntime,
		Title:          "保存项目生命周期脚本",
		Summary:        "创建或纯增量修改项目生命周期脚本（ProjectHookConfig）",
		Tags:           []string{"hooks", "lifecycle"},
		AliasesZH:      []string{"创建生命周期脚本", "修改钩子脚本", "保存 Hook"},
		AliasesEN:      []string{"create hook", "update hook", "save lifecycle hook"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionProjectManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "runtime_list_hooks",
				Family:         "resource",
				IDFrom:         "result.id",
				ReadIDArgument: "hook",
				Expect:         "present",
			},
		},
		Preview: h.aiPreviewRuntimeSaveHook,
		Execute: h.aiExecuteRuntimeSaveHook,
	})

	aitool.Register(reg, aitool.Spec[runtimeDeleteHookInput, map[string]any]{
		Name:           "runtime_delete_hook",
		Domain:         aitool.DomainRuntime,
		Title:          "删除项目生命周期脚本",
		Summary:        "删除项目生命周期脚本并自动解绑关联部署目标",
		Tags:           []string{"hooks", "lifecycle"},
		AliasesZH:      []string{"删除生命周期脚本", "移除钩子脚本", "删除 Hook"},
		AliasesEN:      []string{"delete hook", "remove lifecycle hook"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionProjectManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "runtime_list_hooks",
				Family:         "resource",
				IDFrom:         "result.id",
				ReadIDArgument: "hook",
				Expect:         "absent",
			},
		},
		Preview: h.aiPreviewRuntimeDeleteHook,
		Execute: h.aiExecuteRuntimeDeleteHook,
	})
}

func hookDetailResponseFrom(hook model.ProjectHookConfig) runtimeHookDetailResponse {
	return runtimeHookDetailResponse{
		ID:             hook.ID,
		ProjectID:      hook.ProjectID,
		Name:           hook.Name,
		Script:         hook.Script,
		ScriptBytes:    len(hook.Script),
		ScriptLines:    countScriptLines(hook.Script),
		Shell:          hook.Shell,
		TimeoutSeconds: hook.TimeoutSeconds,
		FailurePolicy:  hook.FailurePolicy,
		CreatedBy:      hook.CreatedBy,
		CreatedAt:      hook.CreatedAt,
		UpdatedAt:      hook.UpdatedAt,
	}
}

func (h *Handler) aiRuntimeListHooks(tc *aitool.ToolContext, in runtimeListHooksInput) (map[string]any, error) {
	_, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectRead)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	if in.Hook != nil && strings.TrimSpace(*in.Hook) != "" {
		hook, err := h.resolveProjectHookForAI(db, project.ID, *in.Hook)
		if err != nil {
			return nil, err
		}
		detail := hookDetailResponseFrom(hook)
		return map[string]any{
			"id":             detail.ID,
			"projectId":      project.ID,
			"name":           detail.Name,
			"script":         detail.Script,
			"scriptBytes":    detail.ScriptBytes,
			"scriptLines":    detail.ScriptLines,
			"shell":          detail.Shell,
			"timeoutSeconds": detail.TimeoutSeconds,
			"failurePolicy":  detail.FailurePolicy,
			"createdBy":      detail.CreatedBy,
			"createdAt":      detail.CreatedAt,
			"updatedAt":      detail.UpdatedAt,
		}, nil
	}

	sortBy := stringOrDefault(in.SortBy, "createdAt")
	switch sortBy {
	case "createdAt", "name":
	default:
		return nil, aitool.BadRequest("argument_invalid", "sortBy must be createdAt or name")
	}
	sortOrder := strings.ToLower(stringOrDefault(in.SortOrder, "desc"))
	if sortOrder != "asc" && sortOrder != "desc" {
		return nil, aitool.BadRequest("argument_invalid", "sortOrder must be asc or desc")
	}
	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)

	query := db.Model(&model.ProjectHookConfig{}).Where("project_id = ?", project.ID)
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	orderCol := "created_at"
	if sortBy == "name" {
		orderCol = "name"
	}
	var hooks []model.ProjectHookConfig
	if err := query.Order(orderCol + " " + sortOrder + ", id asc").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&hooks).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	items := make([]runtimeHookSummaryItem, 0, len(hooks))
	for _, hook := range hooks {
		preview, truncated := truncateUTF8Runes(hook.Script, maxHookScriptPreviewRunes)
		items = append(items, runtimeHookSummaryItem{
			ID:              hook.ID,
			ProjectID:       project.ID,
			Name:            hook.Name,
			ScriptBytes:     len(hook.Script),
			ScriptLines:     countScriptLines(hook.Script),
			ScriptPreview:   preview,
			ScriptTruncated: truncated,
			Shell:           hook.Shell,
			TimeoutSeconds:  hook.TimeoutSeconds,
			FailurePolicy:   hook.FailurePolicy,
			CreatedBy:       hook.CreatedBy,
			CreatedAt:       hook.CreatedAt,
			UpdatedAt:       hook.UpdatedAt,
		})
	}

	env := aitool.PageEnvelope(items, page, pageSize, sortBy, sortOrder, total)
	env["projectId"] = project.ID
	return env, nil
}

func (h *Handler) aiRuntimeListHookRuns(tc *aitool.ToolContext, in runtimeListHookRunsInput) (map[string]any, error) {
	_, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectRead)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	if in.HookRunID != nil && strings.TrimSpace(*in.HookRunID) != "" {
		runID := strings.TrimSpace(*in.HookRunID)
		var run model.HookRun
		if err := db.First(&run, "id = ? AND project_id = ?", runID, project.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, aitool.NotFound("hook_run.not_found", "hook run was not found")
			}
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}

		tailBytes := defaultHookRunLogTailBytes
		if in.LogTailBytes != nil {
			if *in.LogTailBytes < 1 || *in.LogTailBytes > maxHookRunLogTailBytes {
				return nil, aitool.BadRequest("argument_invalid", "logTailBytes must be between 1 and 32768")
			}
			tailBytes = *in.LogTailBytes
		}

		var runLog model.HookRunLog
		if err := db.First(&runLog, "hook_run_id = ? AND project_id = ?", run.ID, project.ID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}

		snapshot, snapshotTruncated := truncateUTF8HeadBytes(run.ScriptSnapshot, maxHookRunSnapshotBytes)
		logContent, logTruncated := truncateUTF8TailBytes(runLog.Content, tailBytes)

		return map[string]any{
			"id":                      run.ID,
			"projectId":               project.ID,
			"hookConfigId":            run.HookConfigID,
			"buildRunId":              run.BuildRunID,
			"buildJobId":              run.BuildJobID,
			"releaseId":               run.ReleaseID,
			"applicationId":           run.ApplicationID,
			"deploymentTargetId":      run.DeploymentTargetID,
			"name":                    run.Name,
			"phase":                   run.Phase,
			"status":                  run.Status,
			"scriptSnapshot":          snapshot,
			"scriptSnapshotBytes":     len(run.ScriptSnapshot),
			"scriptSnapshotTruncated": snapshotTruncated,
			"shell":                   run.Shell,
			"imageRef":                run.ImageRef,
			"timeoutSeconds":          run.TimeoutSeconds,
			"failurePolicy":           run.FailurePolicy,
			"exitCode":                run.ExitCode,
			"message":                 run.Message,
			"log":                     logContent,
			"logBytes":                len(runLog.Content),
			"logTruncated":            logTruncated,
			"startedAt":               run.StartedAt,
			"finishedAt":              run.FinishedAt,
			"createdAt":               run.CreatedAt,
			"updatedAt":               run.UpdatedAt,
		}, nil
	}

	query := db.Model(&model.HookRun{}).Where("project_id = ?", project.ID)
	if in.Phase != nil && strings.TrimSpace(*in.Phase) != "" {
		phase := normalizeHookPhase(*in.Phase)
		if phase == "" {
			return nil, aitool.BadRequest("argument_invalid", "invalid hook phase")
		}
		query = query.Where("phase = ?", phase)
	}
	if in.Status != nil && strings.TrimSpace(*in.Status) != "" {
		status := strings.TrimSpace(*in.Status)
		switch status {
		case "queued", "running", "succeeded", "failed", "canceled":
			query = query.Where("status = ?", status)
		default:
			return nil, aitool.BadRequest("argument_invalid", "status must be queued, running, succeeded, failed, or canceled")
		}
	}
	if in.BuildRunID != nil && strings.TrimSpace(*in.BuildRunID) != "" {
		query = query.Where("build_run_id = ?", strings.TrimSpace(*in.BuildRunID))
	}
	if in.ReleaseID != nil && strings.TrimSpace(*in.ReleaseID) != "" {
		query = query.Where("release_id = ?", strings.TrimSpace(*in.ReleaseID))
	}

	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var runs []model.HookRun
	if err := query.Order("created_at desc, id desc").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&runs).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	items := make([]runtimeHookRunListItem, 0, len(runs))
	for _, r := range runs {
		items = append(items, runtimeHookRunListItem{
			ID:                  r.ID,
			ProjectID:           project.ID,
			HookConfigID:        r.HookConfigID,
			BuildRunID:          r.BuildRunID,
			BuildJobID:          r.BuildJobID,
			ReleaseID:           r.ReleaseID,
			ApplicationID:       r.ApplicationID,
			DeploymentTargetID:  r.DeploymentTargetID,
			Name:                r.Name,
			Phase:               r.Phase,
			Status:              r.Status,
			ScriptSnapshotBytes: len(r.ScriptSnapshot),
			Shell:               r.Shell,
			ImageRef:            r.ImageRef,
			TimeoutSeconds:      r.TimeoutSeconds,
			FailurePolicy:       r.FailurePolicy,
			ExitCode:            r.ExitCode,
			Message:             r.Message,
			StartedAt:           r.StartedAt,
			FinishedAt:          r.FinishedAt,
			CreatedAt:           r.CreatedAt,
			UpdatedAt:           r.UpdatedAt,
		})
	}

	env := aitool.PageEnvelope(items, page, pageSize, "createdAt", "desc", total)
	env["projectId"] = project.ID
	return env, nil
}

func prepareHookSaveDraft(existing *model.ProjectHookConfig, userID, projectID string, in runtimeSaveHookInput) (model.ProjectHookConfig, []aitool.PreviewChange, error) {
	isCreate := existing == nil
	var draft model.ProjectHookConfig
	if isCreate {
		draft = model.ProjectHookConfig{
			ID:             id.New("hook"),
			ProjectID:      projectID,
			Shell:          "sh",
			TimeoutSeconds: 300,
			FailurePolicy:  "fail",
			CreatedBy:      userID,
		}
	} else {
		draft = *existing
	}

	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		if trimmed == "" {
			return model.ProjectHookConfig{}, nil, aitool.BadRequest("argument_invalid", "hook name cannot be empty")
		}
		draft.Name = trimmed
	} else if isCreate {
		return model.ProjectHookConfig{}, nil, aitool.BadRequest("argument_invalid", "name is required when creating a hook")
	}

	if in.Script != nil {
		trimmed := strings.TrimSpace(*in.Script)
		if trimmed == "" {
			return model.ProjectHookConfig{}, nil, aitool.BadRequest("argument_invalid", "script cannot be empty")
		}
		if len(trimmed) > maxHookScriptBytes {
			return model.ProjectHookConfig{}, nil, aitool.BadRequest("argument_invalid", "script exceeds maximum size of 65536 bytes")
		}
		draft.Script = trimmed
	} else if isCreate {
		return model.ProjectHookConfig{}, nil, aitool.BadRequest("argument_invalid", "script is required when creating a hook")
	}

	if in.Shell != nil {
		shell := strings.TrimSpace(*in.Shell)
		switch shell {
		case "sh", "bash":
			draft.Shell = shell
		default:
			return model.ProjectHookConfig{}, nil, aitool.BadRequest("argument_invalid", "shell must be sh or bash")
		}
	}

	if in.TimeoutSeconds != nil {
		if *in.TimeoutSeconds < 1 || *in.TimeoutSeconds > 3600 {
			return model.ProjectHookConfig{}, nil, aitool.BadRequest("argument_invalid", "timeoutSeconds must be between 1 and 3600")
		}
		draft.TimeoutSeconds = *in.TimeoutSeconds
	}

	if in.FailurePolicy != nil {
		fp := strings.TrimSpace(*in.FailurePolicy)
		switch fp {
		case "fail", "ignore":
			draft.FailurePolicy = fp
		default:
			return model.ProjectHookConfig{}, nil, aitool.BadRequest("argument_invalid", "failurePolicy must be fail or ignore")
		}
	}

	var changes []aitool.PreviewChange
	if isCreate {
		changes = append(changes,
			aitool.PreviewChange{Field: "name", After: draft.Name, Action: "create"},
			aitool.PreviewChange{Field: "shell", After: draft.Shell, Action: "create"},
			aitool.PreviewChange{Field: "timeoutSeconds", After: draft.TimeoutSeconds, Action: "create"},
			aitool.PreviewChange{Field: "failurePolicy", After: draft.FailurePolicy, Action: "create"},
			aitool.PreviewChange{Field: "scriptBytes", After: len(draft.Script), Action: "create"},
		)
	} else {
		if draft.Name != existing.Name {
			changes = append(changes, aitool.PreviewChange{Field: "name", Before: existing.Name, After: draft.Name, Action: "update"})
		}
		if draft.Shell != existing.Shell {
			changes = append(changes, aitool.PreviewChange{Field: "shell", Before: existing.Shell, After: draft.Shell, Action: "update"})
		}
		if draft.TimeoutSeconds != existing.TimeoutSeconds {
			changes = append(changes, aitool.PreviewChange{Field: "timeoutSeconds", Before: existing.TimeoutSeconds, After: draft.TimeoutSeconds, Action: "update"})
		}
		if draft.FailurePolicy != existing.FailurePolicy {
			changes = append(changes, aitool.PreviewChange{Field: "failurePolicy", Before: existing.FailurePolicy, After: draft.FailurePolicy, Action: "update"})
		}
		if draft.Script != existing.Script {
			changes = append(changes, aitool.PreviewChange{Field: "scriptBytes", Before: len(existing.Script), After: len(draft.Script), Action: "update"})
		}
	}
	return draft, changes, nil
}

func (h *Handler) aiPreviewRuntimeSaveHook(tc *aitool.ToolContext, in runtimeSaveHookInput) (*aitool.PreviewResult, error) {
	user, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	var existing *model.ProjectHookConfig
	resolvedID := ""
	if in.Hook != nil && strings.TrimSpace(*in.Hook) != "" {
		found, resolveErr := h.resolveProjectHookForAI(db, project.ID, *in.Hook)
		if resolveErr != nil {
			return nil, resolveErr
		}
		existing = &found
		resolvedID = found.ID
	}

	draft, changes, err := prepareHookSaveDraft(existing, user.ID, project.ID, in)
	if err != nil {
		return nil, err
	}

	summary := fmt.Sprintf("Create lifecycle hook %q in project %s", draft.Name, project.Identifier)
	if existing != nil {
		summary = fmt.Sprintf("Update lifecycle hook %q (%s) in project %s", draft.Name, existing.ID, project.Identifier)
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        summary,
			TargetResource: "project_hook",
			TargetID:       resolvedID,
			ProjectID:      project.ID,
			Changes:        changes,
		},
		LockedPayload: lockedHookMutation{
			ProjectID:      project.ID,
			ResolvedHookID: resolvedID,
		},
	}, nil
}

func (h *Handler) aiExecuteRuntimeSaveHook(tc *aitool.ToolContext, in runtimeSaveHookInput) (runtimeHookDetailResponse, error) {
	user, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return runtimeHookDetailResponse{}, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return runtimeHookDetailResponse{}, err
	}
	db := h.aiDB(tc)

	var locked lockedHookMutation
	_ = tc.UnmarshalLockedPayload(&locked)

	var existing *model.ProjectHookConfig
	if locked.ResolvedHookID != "" {
		var found model.ProjectHookConfig
		if err := db.First(&found, "id = ? AND project_id = ?", locked.ResolvedHookID, project.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return runtimeHookDetailResponse{}, aitool.NotFound("project_hook.not_found", "project hook was not found")
			}
			return runtimeHookDetailResponse{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		existing = &found
	} else if in.Hook != nil && strings.TrimSpace(*in.Hook) != "" {
		found, resolveErr := h.resolveProjectHookForAI(db, project.ID, *in.Hook)
		if resolveErr != nil {
			return runtimeHookDetailResponse{}, resolveErr
		}
		existing = &found
	}

	draft, _, err := prepareHookSaveDraft(existing, user.ID, project.ID, in)
	if err != nil {
		return runtimeHookDetailResponse{}, err
	}

	if existing == nil {
		if err := db.Create(&draft).Error; err != nil {
			return runtimeHookDetailResponse{}, aitool.BadRequest("project_hook.save_failed", err.Error())
		}
		h.auditWithContext(user.ID, "project_hook.create", draft.ID, true, draft.Name, tc.Context())
	} else {
		draft.CreatedBy = existing.CreatedBy
		draft.CreatedAt = existing.CreatedAt
		if err := db.Save(&draft).Error; err != nil {
			return runtimeHookDetailResponse{}, aitool.BadRequest("project_hook.save_failed", err.Error())
		}
		h.auditWithContext(user.ID, "project_hook.update", draft.ID, true, draft.Name, tc.Context())
	}
	return hookDetailResponseFrom(draft), nil
}

func (h *Handler) aiPreviewRuntimeDeleteHook(tc *aitool.ToolContext, in runtimeDeleteHookInput) (*aitool.PreviewResult, error) {
	_, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	hook, err := h.resolveProjectHookForAI(db, project.ID, in.Hook)
	if err != nil {
		return nil, err
	}

	var bindingCount int64
	if err := db.Model(&model.DeploymentTargetHookBinding{}).Where("hook_config_id = ?", hook.ID).Count(&bindingCount).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var impact []string
	if bindingCount > 0 {
		impact = append(impact, fmt.Sprintf("Unbinds hook from %d deployment target binding(s)", bindingCount))
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Delete lifecycle hook %q (%s) from project %s", hook.Name, hook.ID, project.Identifier),
			TargetResource: "project_hook",
			TargetID:       hook.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "hook", Before: hook.Name, Action: "delete"},
			},
			Impact: impact,
		},
		LockedPayload: lockedHookMutation{
			ProjectID:      project.ID,
			ResolvedHookID: hook.ID,
		},
	}, nil
}

func (h *Handler) aiExecuteRuntimeDeleteHook(tc *aitool.ToolContext, in runtimeDeleteHookInput) (map[string]any, error) {
	user, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	var locked lockedHookMutation
	_ = tc.UnmarshalLockedPayload(&locked)

	var hook model.ProjectHookConfig
	if locked.ResolvedHookID != "" {
		if err := db.First(&hook, "id = ? AND project_id = ?", locked.ResolvedHookID, project.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, aitool.NotFound("project_hook.not_found", "project hook was not found")
			}
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
	} else {
		resolved, resolveErr := h.resolveProjectHookForAI(db, project.ID, in.Hook)
		if resolveErr != nil {
			return nil, resolveErr
		}
		hook = resolved
	}

	var unboundBindings int64
	if err := db.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("hook_config_id = ?", hook.ID).Delete(&model.DeploymentTargetHookBinding{})
		if res.Error != nil {
			return res.Error
		}
		unboundBindings = res.RowsAffected
		return tx.Delete(&hook).Error
	}); err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "project_hook.delete_failed", err.Error())
	}

	h.auditWithContext(user.ID, "project_hook.delete", hook.ID, true, hook.Name, tc.Context())
	return map[string]any{
		"id":                 hook.ID,
		"name":               hook.Name,
		"unboundBindingRows": unboundBindings,
		"deleted":            true,
		"projectId":          project.ID,
	}, nil
}
