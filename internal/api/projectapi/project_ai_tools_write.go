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
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"gorm.io/gorm"
)

type workspaceCreateProjectInput struct {
	Identifier          string  `json:"identifier" desc:"项目唯一英文标识（创建后不可修改，映射 K8s Namespace）"`
	Name                string  `json:"name" desc:"项目显示名称"`
	Description         *string `json:"description,omitempty" desc:"项目描述"`
	MaxConcurrentBuilds *int    `json:"maxConcurrentBuilds,omitempty" min:"1" desc:"最大并发构建数"`
	WebConsoleEnabled   *bool   `json:"webConsoleEnabled,omitempty" desc:"项目级 Web 控制台/Exec 安全开关"`
}

type workspaceUpdateProjectInput struct {
	Project             string  `json:"project" desc:"项目 ID 或 identifier"`
	Name                *string `json:"name,omitempty" desc:"新项目显示名称"`
	Description         *string `json:"description,omitempty" desc:"新项目描述（空串或省略保持原值；清空请传 clearDescription: true）"`
	ClearDescription    *bool   `json:"clearDescription,omitempty" desc:"显式清空项目描述"`
	MaxConcurrentBuilds *int    `json:"maxConcurrentBuilds,omitempty" min:"1" desc:"构建并发上限"`
	WebConsoleEnabled   *bool   `json:"webConsoleEnabled,omitempty" desc:"项目级容器远程执行/终端总开关"`
}

type workspacePinProjectInput struct {
	Project string `json:"project" desc:"项目 ID 或 identifier"`
	Pinned  bool   `json:"pinned" desc:"true 收藏置顶，false 取消收藏"`
}

type workspaceReorderDashboardProjectsInput struct {
	Projects []string `json:"projects" required:"true" min:"1" max:"8" desc:"有序项目 ID 或 identifier 数组（1..8 个）"`
}

type workspaceRequestBillingTransferInput struct {
	Project    string `json:"project" desc:"项目 ID 或 identifier"`
	TargetUser string `json:"targetUser" desc:"目标项目成员的用户 ID 或邮箱"`
}

type workspaceDeleteProjectInput struct {
	Project string `json:"project" desc:"项目 ID 或 identifier"`
}

type workspaceSaveMemberInput struct {
	Project string `json:"project" desc:"项目 ID 或 identifier"`
	User    string `json:"user" desc:"用户 ID、成员记录 ID 或唯一邮箱"`
	Role    string `json:"role" enum:"owner|admin|developer|viewer" desc:"项目角色（严格枚举：owner | admin | developer | viewer）"`
}

type workspaceRemoveMemberInput struct {
	Project string `json:"project" desc:"项目 ID 或 identifier"`
	Member  string `json:"member" desc:"成员记录 ID、用户 ID 或唯一邮箱"`
}

type lockedProjectMutation struct {
	ProjectID        string `json:"projectId"`
	RecipientUserID  string `json:"recipientUserId,omitempty"`
	TargetUserID     string `json:"targetUserId,omitempty"`
	ExistingMemberID string `json:"existingMemberId,omitempty"`
}

func (h *Handler) registerWorkspaceWriteAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[workspaceCreateProjectInput, map[string]any]{
		Name:              "workspace_create_project",
		Domain:            aitool.DomainWorkspace,
		Title:             "创建项目空间",
		Summary:           "创建新的项目空间（仅限全局会话调用）",
		Tags:              []string{"projects"},
		AliasesZH:         []string{"新建项目空间", "创建项目"},
		AliasesEN:         []string{"create project", "new project space"},
		GlobalSessionOnly: true,
		ApprovalTier:      aitool.ApprovalTierL1,
		Effect:            aitool.EffectWrite,
		RequiredScopes:    []string{string(authz.ActionProjectWrite)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "workspace_get_project",
				Family:         "resource",
				IDFrom:         "result.projectId",
				ReadIDArgument: "project",
				Expect:         "present",
			},
		},
		Execute: h.aiWorkspaceCreateProject,
	})

	aitool.Register(reg, aitool.Spec[workspaceUpdateProjectInput, map[string]any]{
		Name:           "workspace_update_project",
		Domain:         aitool.DomainWorkspace,
		Title:          "修改项目空间配置",
		Summary:        "增量修改项目名称、描述、构建并发上限或项目级 Web 控制台安全开关",
		Tags:           []string{"projects", "governance"},
		AliasesZH:      []string{"更新项目配置", "修改项目名称", "开关项目 Web 控制台"},
		AliasesEN:      []string{"update project", "patch project settings"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionProjectManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "workspace_get_project",
				Family:         "resource",
				IDFrom:         "result.projectId",
				ReadIDArgument: "project",
				Expect:         "present",
			},
		},
		Preview: h.aiPreviewWorkspaceUpdateProject,
		Execute: h.aiExecuteWorkspaceUpdateProject,
	})

	aitool.Register(reg, aitool.Spec[workspacePinProjectInput, map[string]any]{
		Name:           "workspace_pin_project",
		Domain:         aitool.DomainWorkspace,
		Title:          "收藏或取消收藏项目空间",
		Summary:        "切换当前用户对单个项目的个人收藏置顶状态",
		Tags:           []string{"projects", "pins"},
		AliasesZH:      []string{"置顶项目", "收藏项目", "取消置顶项目"},
		AliasesEN:      []string{"pin project", "unpin project"},
		ApprovalTier:   aitool.ApprovalTierL1,
		Idempotent:     true,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionProjectRead)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "workspace_list_projects",
				Family:         "resource",
				IDFrom:         "result.projectId",
				Collection:     true,
				CollectionPath: "items",
				Expect:         "present",
			},
		},
		Execute: h.aiWorkspacePinProject,
	})

	aitool.Register(reg, aitool.Spec[workspaceReorderDashboardProjectsInput, map[string]any]{
		Name:              "workspace_reorder_my_dashboard_projects",
		Domain:            aitool.DomainWorkspace,
		Title:             "调整个人工作台项目排序",
		Summary:           "调整当前用户个人工作台看板前排项目展示顺序（仅限全局会话调用）",
		Tags:              []string{"projects", "dashboard"},
		AliasesZH:         []string{"调整看板项目顺序", "项目排序"},
		AliasesEN:         []string{"reorder dashboard projects", "update project order"},
		GlobalSessionOnly: true,
		ApprovalTier:      aitool.ApprovalTierL1,
		Idempotent:        true,
		Effect:            aitool.EffectWrite,
		RequiredScopes:    []string{string(authz.ActionProjectRead)},
		Verification: []aitool.OperationVerification{
			{
				OperationID: "workspace_list_projects",
				Family:      "project_order",
				Expect:      "present",
			},
		},
		Execute: h.aiWorkspaceReorderDashboardProjects,
	})

	aitool.Register(reg, aitool.Spec[workspaceRequestBillingTransferInput, map[string]any]{
		Name:           "workspace_request_billing_transfer",
		Domain:         aitool.DomainWorkspace,
		Title:          "发起项目计费归属人转移申请",
		Summary:        "向目标项目成员发起项目计费归属人（Billing Owner）变更确认申请",
		Tags:           []string{"projects", "billing"},
		AliasesZH:      []string{"转移项目计费归属人", "修改项目付款人"},
		AliasesEN:      []string{"request billing owner transfer", "transfer project billing"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionProjectManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "workspace_get_project",
				Family:         "resource",
				IDFrom:         "result.projectId",
				ReadIDArgument: "project",
				Expect:         "present",
			},
		},
		Preview: h.aiPreviewWorkspaceRequestBillingTransfer,
		Execute: h.aiExecuteWorkspaceRequestBillingTransfer,
	})

	aitool.Register(reg, aitool.Spec[workspaceDeleteProjectInput, map[string]any]{
		Name:           "workspace_delete_project",
		Domain:         aitool.DomainWorkspace,
		Title:          "删除项目空间",
		Summary:        "异步销毁整个项目空间及其名下全部应用、环境、路由与托管资源",
		Tags:           []string{"projects"},
		AliasesZH:      []string{"销毁项目空间", "删除项目"},
		AliasesEN:      []string{"delete project", "destroy project space"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectAsync,
		RequiredScopes: []string{string(authz.ActionProjectDelete)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:     "workspace_get_project",
				Family:          "resource",
				IDFrom:          "result.projectId",
				ReadIDArgument:  "project",
				StatusField:     "deleteStatus",
				PendingStatuses: []string{"deleting"},
				FailureStatuses: []string{"delete_failed"},
				Expect:          "absent",
			},
		},
		Preview: h.aiPreviewWorkspaceDeleteProject,
		Execute: h.aiExecuteWorkspaceDeleteProject,
	})

	aitool.Register(reg, aitool.Spec[workspaceSaveMemberInput, projectMemberResponse]{
		Name:           "workspace_save_member",
		Domain:         aitool.DomainWorkspace,
		Title:          "添加或修改项目成员角色",
		Summary:        "向项目空间添加成员或调整现有成员的项目角色",
		Tags:           []string{"members", "iam"},
		AliasesZH:      []string{"添加项目成员", "修改成员角色", "邀请成员"},
		AliasesEN:      []string{"add project member", "update member role", "save project member"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionProjectManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "workspace_list_members",
				Family:         "resource",
				IDFrom:         "result.id",
				ReadIDArgument: "member",
				Collection:     true,
				CollectionPath: "items",
				Expect:         "present",
			},
		},
		Preview: h.aiPreviewWorkspaceSaveMember,
		Execute: h.aiExecuteWorkspaceSaveMember,
	})

	aitool.Register(reg, aitool.Spec[workspaceRemoveMemberInput, map[string]any]{
		Name:           "workspace_remove_member",
		Domain:         aitool.DomainWorkspace,
		Title:          "移除项目成员",
		Summary:        "将指定成员移出项目空间",
		Tags:           []string{"members", "iam"},
		AliasesZH:      []string{"删除项目成员", "移出项目成员"},
		AliasesEN:      []string{"remove project member", "delete member"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionProjectManage)},
		Verification: []aitool.OperationVerification{
			{
				OperationID:    "workspace_list_members",
				Family:         "resource",
				IDFrom:         "result.id",
				ReadIDArgument: "member",
				Collection:     true,
				CollectionPath: "items",
				Expect:         "absent",
			},
		},
		Preview: h.aiPreviewWorkspaceRemoveMember,
		Execute: h.aiExecuteWorkspaceRemoveMember,
	})
}

func (h *Handler) projectResultMap(project model.Project, ctx *aitool.ToolContext) map[string]any {
	resp := h.projectResponse(project, ctx.Context())
	return map[string]any{
		"id":                     project.ID,
		"projectId":              project.ID,
		"identifier":             project.Identifier,
		"kubernetesNamespace":    project.KubernetesNamespace,
		"name":                   project.Name,
		"description":            project.Description,
		"maxConcurrentBuilds":    project.MaxConcurrentBuilds,
		"webConsoleEnabled":      project.WebConsoleEnabled,
		"billingOwnerUserId":     project.BillingOwnerUserID,
		"billingOwner":           resp.BillingOwner,
		"systemKey":              project.SystemKey,
		"networkIngressMode":     project.NetworkIngressMode,
		"networkEgressMode":      project.NetworkEgressMode,
		"networkEgressAllowlist": project.NetworkEgressAllowlist,
		"deleteStatus":           project.DeleteStatus,
		"createdAt":              project.CreatedAt,
		"updatedAt":              project.UpdatedAt,
	}
}

func (h *Handler) aiWorkspaceCreateProject(tc *aitool.ToolContext, in workspaceCreateProjectInput) (map[string]any, error) {
	if strings.TrimSpace(tc.ConversationProjectID) != "" {
		return nil, projectScopeMismatchError("workspace_create_project is only allowed in an unbound global conversation")
	}
	user, err := h.aiActor(tc)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	maxBuilds := 0
	if in.MaxConcurrentBuilds != nil {
		if *in.MaxConcurrentBuilds < 1 {
			return nil, aitool.BadRequest("argument_invalid", "maxConcurrentBuilds must be >= 1")
		}
		maxBuilds = *in.MaxConcurrentBuilds
	}
	desc := ""
	if in.Description != nil {
		desc = strings.TrimSpace(*in.Description)
	}

	project, createErr := projectservice.NewService(db).Create(tc.Context(), user.ID, projectservice.CreateInput{
		Identifier:          in.Identifier,
		Name:                in.Name,
		Description:         desc,
		MaxConcurrentBuilds: maxBuilds,
		WebConsoleEnabled:   in.WebConsoleEnabled,
	})
	switch {
	case errors.Is(createErr, projectservice.ErrIdentifierInvalid):
		return nil, aitool.BadRequest("project.identifier_invalid", createErr.Error())
	case errors.Is(createErr, projectservice.ErrIdentifierExists):
		return nil, aitool.Conflict("project.identifier_exists", "project identifier already exists")
	case errors.Is(createErr, projectservice.ErrIdentifierDeleteInProgress):
		return nil, aitool.Conflict("project.identifier_delete_in_progress", "project with same identifier is currently being deleted")
	case errors.Is(createErr, projectservice.ErrIdentifierDeleteFailed):
		return nil, aitool.Conflict("project.identifier_delete_failed", "previous deletion of project with same identifier failed; complete cleanup first")
	case errors.Is(createErr, projectservice.ErrInputInvalid):
		return nil, aitool.BadRequest("request.invalid", "project input is invalid")
	case createErr != nil:
		return nil, aitool.NewError(http.StatusInternalServerError, "project.create_failed", createErr.Error())
	}

	h.auditWithContext(user.ID, "project.create", project.ID, true, project.Identifier, tc.Context())
	return h.projectResultMap(project, tc), nil
}

func prepareProjectUpdateDraft(existing model.Project, in workspaceUpdateProjectInput) (model.Project, []aitool.PreviewChange, []string, error) {
	updated := existing
	var changes []aitool.PreviewChange
	var warnings []string

	if in.ClearDescription != nil && *in.ClearDescription && in.Description != nil && strings.TrimSpace(*in.Description) != "" {
		return model.Project{}, nil, nil, aitool.BadRequest("argument_invalid", "description and clearDescription=true cannot both be set")
	}

	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		if trimmed == "" {
			return model.Project{}, nil, nil, aitool.BadRequest("argument_invalid", "name cannot be empty")
		}
		if trimmed != existing.Name {
			updated.Name = trimmed
			changes = append(changes, aitool.PreviewChange{Field: "name", Before: existing.Name, After: updated.Name, Action: "update"})
		}
	}

	if in.ClearDescription != nil && *in.ClearDescription {
		if existing.Description != "" {
			updated.Description = ""
			changes = append(changes, aitool.PreviewChange{Field: "description", Before: existing.Description, After: "", Action: "clear"})
		}
	} else if in.Description != nil && strings.TrimSpace(*in.Description) != "" {
		trimmed := strings.TrimSpace(*in.Description)
		if trimmed != existing.Description {
			updated.Description = trimmed
			changes = append(changes, aitool.PreviewChange{Field: "description", Before: existing.Description, After: updated.Description, Action: "update"})
		}
	}

	if in.MaxConcurrentBuilds != nil {
		if *in.MaxConcurrentBuilds < 1 {
			return model.Project{}, nil, nil, aitool.BadRequest("argument_invalid", "maxConcurrentBuilds must be >= 1")
		}
		if *in.MaxConcurrentBuilds != existing.MaxConcurrentBuilds {
			updated.MaxConcurrentBuilds = *in.MaxConcurrentBuilds
			changes = append(changes, aitool.PreviewChange{Field: "maxConcurrentBuilds", Before: existing.MaxConcurrentBuilds, After: updated.MaxConcurrentBuilds, Action: "update"})
		}
	}

	if in.WebConsoleEnabled != nil && *in.WebConsoleEnabled != existing.WebConsoleEnabled {
		updated.WebConsoleEnabled = *in.WebConsoleEnabled
		changes = append(changes, aitool.PreviewChange{Field: "webConsoleEnabled", Before: existing.WebConsoleEnabled, After: updated.WebConsoleEnabled, Action: "update"})
		if *in.WebConsoleEnabled {
			warnings = append(warnings, "Enabling project-level webConsoleEnabled permits container Exec/Terminal on targets whose governance policy allows it.")
		} else {
			warnings = append(warnings, "Disabling project-level webConsoleEnabled immediately blocks container Exec/Terminal across all targets in this project.")
		}
	}

	return updated, changes, warnings, nil
}

func (h *Handler) aiPreviewWorkspaceUpdateProject(tc *aitool.ToolContext, in workspaceUpdateProjectInput) (*aitool.PreviewResult, error) {
	_, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}
	_, changes, warnings, err := prepareProjectUpdateDraft(project, in)
	if err != nil {
		return nil, err
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Update project space %q (%s)", project.Name, project.Identifier),
			TargetResource: "project",
			TargetID:       project.ID,
			ProjectID:      project.ID,
			Changes:        changes,
			Warnings:       warnings,
		},
		LockedPayload: lockedProjectMutation{ProjectID: project.ID},
	}, nil
}

func (h *Handler) aiExecuteWorkspaceUpdateProject(tc *aitool.ToolContext, in workspaceUpdateProjectInput) (map[string]any, error) {
	user, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	updated, _, _, err := prepareProjectUpdateDraft(project, in)
	if err != nil {
		return nil, err
	}
	if err := db.Save(&updated).Error; err != nil {
		return nil, aitool.BadRequest("project.update_failed", err.Error())
	}
	h.auditWithContext(user.ID, "project.update", updated.ID, true, updated.Identifier, tc.Context())
	return h.projectResultMap(updated, tc), nil
}

func (h *Handler) aiWorkspacePinProject(tc *aitool.ToolContext, in workspacePinProjectInput) (map[string]any, error) {
	user, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectPin)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	if in.Pinned {
		now := time.Now()
		var pin model.ProjectPin
		findErr := db.First(&pin, "user_id = ? AND project_id = ?", user.ID, project.ID).Error
		if findErr == nil {
			pin.PinnedAt = now
			if err := db.Save(&pin).Error; err != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "project_pin.save_failed", err.Error())
			}
		} else if errors.Is(findErr, gorm.ErrRecordNotFound) {
			pin = model.ProjectPin{
				ID:        id.New("ppin"),
				UserID:    user.ID,
				ProjectID: project.ID,
				PinnedAt:  now,
			}
			if err := db.Create(&pin).Error; err != nil {
				return nil, aitool.BadRequest("project_pin.save_failed", err.Error())
			}
		} else {
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", findErr.Error())
		}
	} else {
		if err := db.Delete(&model.ProjectPin{}, "user_id = ? AND project_id = ?", user.ID, project.ID).Error; err != nil {
			return nil, aitool.NewError(http.StatusInternalServerError, "project_pin.delete_failed", err.Error())
		}
	}

	return map[string]any{
		"id":         project.ID,
		"projectId":  project.ID,
		"identifier": project.Identifier,
		"pinned":     in.Pinned,
	}, nil
}

func (h *Handler) aiWorkspaceReorderDashboardProjects(tc *aitool.ToolContext, in workspaceReorderDashboardProjectsInput) (map[string]any, error) {
	if strings.TrimSpace(tc.ConversationProjectID) != "" {
		return nil, projectScopeMismatchError("workspace_reorder_my_dashboard_projects is only allowed in an unbound global conversation")
	}
	user, err := h.aiActor(tc)
	if err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	rawRefs := normalizedProjectOrderIDs(in.Projects)
	if len(rawRefs) == 0 {
		return nil, aitool.BadRequest("argument_invalid", "projects must contain at least 1 project")
	}
	if len(rawRefs) > 8 {
		return nil, aitool.BadRequest("argument_invalid", "at most 8 dashboard projects can be ordered")
	}

	seenIDs := make(map[string]struct{}, len(rawRefs))
	projectIDs := make([]string, 0, len(rawRefs))
	for _, ref := range rawRefs {
		proj, resolveErr := aitool.ResolveProject(db, ref)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if _, dup := seenIDs[proj.ID]; dup {
			continue
		}
		seenIDs[proj.ID] = struct{}{}
		allowed, checkErr := h.projectActionAllowed(tc.Context(), authz.ProjectSubject{
			UserID:       user.ID,
			PlatformRole: authz.PlatformRoleUser,
		}, proj.ID, authz.ActionProjectPin)
		if checkErr != nil {
			return nil, mapProjectAuthErrorForAI(checkErr)
		}
		if !allowed {
			return nil, aitool.Forbidden("auth.forbidden", fmt.Sprintf("you are not a member of project %q", proj.Identifier))
		}
		projectIDs = append(projectIDs, proj.ID)
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ProjectMember{}).
			Where("user_id = ? AND project_id NOT IN ?", user.ID, projectIDs).
			Update("dashboard_order", 0).Error; err != nil {
			return err
		}
		for idx, pid := range projectIDs {
			if err := tx.Model(&model.ProjectMember{}).
				Where("user_id = ? AND project_id = ?", user.ID, pid).
				Update("dashboard_order", idx+1).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "project_order.update_failed", err.Error())
	}

	return map[string]any{
		"projectIds": projectIDs,
	}, nil
}

func (h *Handler) resolveBillingTransferTargetForAI(tc *aitool.ToolContext, user model.User, project model.Project, targetUserRef string) (model.User, error) {
	db := h.aiDB(tc)
	if project.BillingOwnerUserID != user.ID {
		return model.User{}, aitool.Forbidden("inbox.billing_owner_transfer_forbidden", "only the current billingOwner can request a billing owner transfer")
	}
	isOwner, err := h.projectMemberOwnerAllowedForAI(tc, project.ID, user.ID)
	if err != nil {
		return model.User{}, err
	}
	if !isOwner {
		return model.User{}, aitool.Forbidden("inbox.billing_owner_transfer_forbidden", "caller must also be an owner member of this project")
	}

	ref := strings.TrimSpace(targetUserRef)
	if ref == "" {
		return model.User{}, aitool.BadRequest("argument_invalid", "targetUser is required")
	}

	var targetUser model.User
	if err := db.Where("(id = ? OR lower(email) = ?) AND disabled = ?", ref, strings.ToLower(ref), false).
		Order(gorm.Expr("CASE WHEN id = ? THEN 0 ELSE 1 END", ref)).
		First(&targetUser).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.User{}, aitool.NotFound("user.not_found", "target user was not found or is disabled")
		}
		return model.User{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	if targetUser.ID == user.ID || targetUser.ID == project.BillingOwnerUserID {
		return model.User{}, aitool.Conflict("inbox.billing_owner_transfer_conflict", "target user is already the billing owner of this project")
	}

	var member model.ProjectMember
	if err := db.First(&member, "project_id = ? AND user_id = ?", project.ID, targetUser.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.User{}, aitool.NewErrorWithHint(
				http.StatusBadRequest,
				"inbox.billing_owner_transfer_invalid",
				"target user is not a member of this project",
				"Add the target user to the project first via workspace_save_member before requesting billing transfer.",
			)
		}
		return model.User{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var pending int64
	if err := db.Model(&model.InboxActionRequest{}).
		Where("type = ? AND project_id = ? AND status IN ?", projectservice.BillingOwnerTransferRequestType, project.ID, []string{"pending", "processing"}).
		Count(&pending).Error; err != nil {
		return model.User{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if pending > 0 {
		return model.User{}, aitool.Conflict("inbox.billing_owner_transfer_conflict", "a pending billing owner transfer request already exists for this project")
	}

	return targetUser, nil
}

func (h *Handler) aiPreviewWorkspaceRequestBillingTransfer(tc *aitool.ToolContext, in workspaceRequestBillingTransferInput) (*aitool.PreviewResult, error) {
	user, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectOwnerOnly)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}
	targetUser, err := h.resolveBillingTransferTargetForAI(tc, user, project, in.TargetUser)
	if err != nil {
		return nil, err
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Request billing owner transfer for project %q to %s (%s)", project.Name, targetUser.Name, targetUser.Email),
			TargetResource: "project",
			TargetID:       project.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "billingOwnerUserId", Before: project.BillingOwnerUserID, After: targetUser.ID, Action: "transfer_request"},
			},
			Impact: []string{"Sends a high-priority confirmation request to the target member's inbox; billing ownership transfers upon their acceptance."},
		},
		LockedPayload: lockedProjectMutation{
			ProjectID:       project.ID,
			RecipientUserID: targetUser.ID,
		},
	}, nil
}

func (h *Handler) aiExecuteWorkspaceRequestBillingTransfer(tc *aitool.ToolContext, in workspaceRequestBillingTransferInput) (map[string]any, error) {
	user, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectOwnerOnly)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	var locked lockedProjectMutation
	_ = tc.UnmarshalLockedPayload(&locked)
	recipientID := strings.TrimSpace(locked.RecipientUserID)
	if recipientID == "" {
		targetUser, resolveErr := h.resolveBillingTransferTargetForAI(tc, user, project, in.TargetUser)
		if resolveErr != nil {
			return nil, resolveErr
		}
		recipientID = targetUser.ID
	}

	request, reqErr := (projectservice.BillingOwnerTransferService{DB: db}).Request(tc.Context(), user.ID, project.ID, recipientID)
	if reqErr != nil {
		return nil, mapInboxTransferErrorForAI(reqErr)
	}
	defaultInboxBroker.Notify(request.RecipientUserID, "")

	return map[string]any{
		"id":              request.ID,
		"requestId":       request.ID,
		"projectId":       project.ID,
		"requesterUserId": request.RequesterUserID,
		"recipientUserId": request.RecipientUserID,
		"status":          request.Status,
		"rowVersion":      request.RowVersion,
		"expiresAt":       request.ExpiresAt,
		"createdAt":       request.CreatedAt,
	}, nil
}

func (h *Handler) aiPreviewWorkspaceDeleteProject(tc *aitool.ToolContext, in workspaceDeleteProjectInput) (*aitool.PreviewResult, error) {
	_, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectDelete)
	if err != nil {
		return nil, err
	}
	if isSystemProject(project) {
		return nil, aitool.Forbidden("project.system_protected", "platform system project space cannot be deleted")
	}
	if !deleteStatusCanStart(project.DeleteStatus) {
		return nil, aitool.Conflict("project.delete_in_progress", "project space is already being deleted")
	}
	db := h.aiDB(tc)

	var appCount, targetCount, routeCount, volumeCount int64
	_ = db.Model(&model.Application{}).Where("project_id = ? AND delete_status NOT IN ?", project.ID, projectOverviewExcludedApplicationDeleteStatuses).Count(&appCount).Error
	_ = db.Model(&model.DeploymentTarget{}).Where("project_id = ? AND delete_status NOT IN ?", project.ID, projectOverviewExcludedApplicationDeleteStatuses).Count(&targetCount).Error
	_ = db.Model(&model.GatewayRoute{}).Where("project_id = ? AND delete_status NOT IN ?", project.ID, projectOverviewExcludedRouteDeleteStatuses).Count(&routeCount).Error
	_ = db.Model(&model.ProjectVolume{}).Where("project_id = ?", project.ID).Count(&volumeCount).Error

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Permanently destroy project space %q (%s)", project.Name, project.Identifier),
			TargetResource: "project",
			TargetID:       project.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "deleteStatus", Before: project.DeleteStatus, After: "deleting", Action: "delete"},
			},
			Impact: []string{
				fmt.Sprintf("Asynchronously destroys %d application(s), %d deployment target(s), %d gateway route(s), and %d project volume(s)", appCount, targetCount, routeCount, volumeCount),
			},
			Warnings: []string{"This operation is irreversible and permanently destroys all workloads, routes, and managed storage in this project space."},
		},
		LockedPayload: lockedProjectMutation{ProjectID: project.ID},
	}, nil
}

func (h *Handler) aiExecuteWorkspaceDeleteProject(tc *aitool.ToolContext, in workspaceDeleteProjectInput) (map[string]any, error) {
	user, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectDelete)
	if err != nil {
		return nil, err
	}
	if isSystemProject(project) {
		return nil, aitool.Forbidden("project.system_protected", "platform system project space cannot be deleted")
	}
	if !deleteStatusCanStart(project.DeleteStatus) {
		return nil, aitool.Conflict("project.delete_in_progress", "project space is already being deleted")
	}
	db := h.aiDB(tc)

	if err := markResourceDeleting(db, &model.Project{}, project.ID); err != nil {
		if errors.Is(err, errResourceDeleteAlreadyStarted) {
			return nil, aitool.Conflict("project.delete_in_progress", "project space is already being deleted")
		}
		return nil, aitool.NewError(http.StatusInternalServerError, "project.delete_failed", err.Error())
	}
	if !h.enqueueResourceCleanup(tc.Context(), "project", project.ID, project.ID, user.ID) {
		_ = markResourceDeleteFailed(db, &model.Project{}, project.ID, "failed to enqueue project cleanup task")
		h.auditWithContext(user.ID, "project.delete", project.ID, false, "cleanup_enqueue_failed", tc.Context())
		return nil, aitool.NewError(http.StatusServiceUnavailable, "project.cleanup_enqueue_failed", "failed to enqueue project resource cleanup task; please retry later")
	}
	h.auditWithContext(user.ID, "project.delete", project.ID, true, "cleanup_queued", tc.Context())

	return map[string]any{
		"id":           project.ID,
		"projectId":    project.ID,
		"deleteStatus": "deleting",
	}, nil
}

func resolveMemberOrCandidateUserForAI(db *gorm.DB, projectID, ref string) (model.User, *model.ProjectMember, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.User{}, nil, aitool.BadRequest("argument_invalid", "user is required")
	}

	var existingMember model.ProjectMember
	if err := db.Table("project_members").
		Joins("join users on users.id = project_members.user_id").
		Where("project_members.project_id = ? AND (project_members.id = ? OR project_members.user_id = ? OR lower(users.email) = ?)",
			projectID, ref, ref, strings.ToLower(ref)).
		Select("project_members.*").
		First(&existingMember).Error; err == nil {
		var u model.User
		if userErr := db.First(&u, "id = ?", existingMember.UserID).Error; userErr != nil {
			return model.User{}, nil, aitool.NotFound("user.not_found", "member user record was not found")
		}
		return u, &existingMember, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.User{}, nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var targetUser model.User
	if err := db.Where("(id = ? OR lower(email) = ?) AND disabled = ?", ref, strings.ToLower(ref), false).
		Order(gorm.Expr("CASE WHEN id = ? THEN 0 ELSE 1 END", ref)).
		First(&targetUser).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.User{}, nil, aitool.NotFound("user.not_found", "user was not found or is disabled")
		}
		return model.User{}, nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	return targetUser, nil, nil
}

func (h *Handler) validateMemberSaveRulesForAI(tc *aitool.ToolContext, actor model.User, project model.Project, existing *model.ProjectMember, role string) error {
	if !authz.IsProjectRole(role) {
		return aitool.BadRequest("argument_invalid", "role must be owner, admin, developer, or viewer")
	}
	touchesOwner := role == authz.ProjectRoleOwner || (existing != nil && existing.Role == authz.ProjectRoleOwner)
	if touchesOwner {
		actorIsOwner, err := h.projectMemberOwnerAllowedForAI(tc, project.ID, actor.ID)
		if err != nil {
			return err
		}
		if !actorIsOwner {
			return aitool.Forbidden("auth.forbidden", "only a real project owner member can grant or modify the owner role")
		}
	}
	if existing != nil && existing.Role == authz.ProjectRoleOwner && role != authz.ProjectRoleOwner {
		if !h.projectHasAnotherOwner(tc.Context(), project.ID, existing.ID) {
			return aitool.BadRequest("project_member.last_owner_protected", "at least one owner member must remain in the project")
		}
	}
	return nil
}

func (h *Handler) aiPreviewWorkspaceSaveMember(tc *aitool.ToolContext, in workspaceSaveMemberInput) (*aitool.PreviewResult, error) {
	actor, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}
	role := strings.TrimSpace(in.Role)
	db := h.aiDB(tc)

	targetUser, existing, err := resolveMemberOrCandidateUserForAI(db, project.ID, in.User)
	if err != nil {
		return nil, err
	}
	if err := h.validateMemberSaveRulesForAI(tc, actor, project, existing, role); err != nil {
		return nil, err
	}

	var changes []aitool.PreviewChange
	summary := ""
	existingMemberID := ""
	if existing == nil {
		summary = fmt.Sprintf("Add %s (%s) to project %q as %s", targetUser.Name, targetUser.Email, project.Name, role)
		changes = append(changes, aitool.PreviewChange{Field: "role", After: role, Action: "create"})
	} else {
		existingMemberID = existing.ID
		summary = fmt.Sprintf("Update role of %s (%s) in project %q from %s to %s", targetUser.Name, targetUser.Email, project.Name, existing.Role, role)
		changes = append(changes, aitool.PreviewChange{Field: "role", Before: existing.Role, After: role, Action: "update"})
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        summary,
			TargetResource: "project_member",
			TargetID:       existingMemberID,
			ProjectID:      project.ID,
			Changes:        changes,
		},
		LockedPayload: lockedProjectMutation{
			ProjectID:        project.ID,
			TargetUserID:     targetUser.ID,
			ExistingMemberID: existingMemberID,
		},
	}, nil
}

func (h *Handler) aiExecuteWorkspaceSaveMember(tc *aitool.ToolContext, in workspaceSaveMemberInput) (projectMemberResponse, error) {
	actor, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return projectMemberResponse{}, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return projectMemberResponse{}, err
	}
	role := strings.TrimSpace(in.Role)
	db := h.aiDB(tc)

	targetUser, existing, err := resolveMemberOrCandidateUserForAI(db, project.ID, in.User)
	if err != nil {
		return projectMemberResponse{}, err
	}
	if err := h.validateMemberSaveRulesForAI(tc, actor, project, existing, role); err != nil {
		return projectMemberResponse{}, err
	}

	if existing == nil {
		member := model.ProjectMember{
			ID:        id.New("mem"),
			ProjectID: project.ID,
			UserID:    targetUser.ID,
			Role:      role,
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&member).Error; err != nil {
				return err
			}
			return publishProjectMemberInbox(tc.Context(), tx, projectMemberInboxInput{
				Type: "project.member_added", Priority: "normal", Project: project, Actor: actor,
				RecipientUserID: targetUser.ID, MemberID: member.ID, Role: member.Role,
				DedupKey: "project-member-added:" + member.ID,
			})
		}); err != nil {
			return projectMemberResponse{}, aitool.NewError(http.StatusInternalServerError, "inbox.operation_failed", "project member save failed")
		}
		h.auditWithContext(actor.ID, "project_member.create", member.ID, true, member.Role+" "+runtimecluster.EgressExceptionAuditMarker(db, member.ProjectID), tc.Context())
		defaultInboxBroker.Notify(targetUser.ID, "")
		return projectMemberResponse{
			ID:        member.ID,
			ProjectID: project.ID,
			UserID:    targetUser.ID,
			Role:      member.Role,
			Email:     targetUser.Email,
			Name:      targetUser.Name,
		}, nil
	}

	previousRole := existing.Role
	if previousRole != role {
		existing.Role = role
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Save(existing).Error; err != nil {
				return err
			}
			return publishProjectMemberInbox(tc.Context(), tx, projectMemberInboxInput{
				Type: "project.member_role_changed", Priority: "high", Project: project, Actor: actor,
				RecipientUserID: existing.UserID, MemberID: existing.ID, Role: existing.Role, PreviousRole: previousRole,
			})
		}); err != nil {
			return projectMemberResponse{}, aitool.NewError(http.StatusInternalServerError, "inbox.operation_failed", "project member update failed")
		}
		h.auditWithContext(actor.ID, "project_member.update", existing.ID, true, existing.Role+" "+runtimecluster.EgressExceptionAuditMarker(db, existing.ProjectID), tc.Context())
		defaultInboxBroker.Notify(existing.UserID, "")
	}

	return projectMemberResponse{
		ID:        existing.ID,
		ProjectID: project.ID,
		UserID:    existing.UserID,
		Role:      existing.Role,
		Email:     targetUser.Email,
		Name:      targetUser.Name,
	}, nil
}

func resolveExistingProjectMemberForAI(db *gorm.DB, projectID, ref string) (model.ProjectMember, model.User, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.ProjectMember{}, model.User{}, aitool.BadRequest("argument_invalid", "member is required")
	}
	var member model.ProjectMember
	if err := db.Table("project_members").
		Joins("join users on users.id = project_members.user_id").
		Where("project_members.project_id = ? AND (project_members.id = ? OR project_members.user_id = ? OR lower(users.email) = ?)",
			projectID, ref, ref, strings.ToLower(ref)).
		Select("project_members.*").
		First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.ProjectMember{}, model.User{}, aitool.NotFound("project_member.not_found", "project member was not found")
		}
		return model.ProjectMember{}, model.User{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	var user model.User
	_ = db.First(&user, "id = ?", member.UserID).Error
	return member, user, nil
}

func (h *Handler) validateMemberRemovalForAI(tc *aitool.ToolContext, actor model.User, project model.Project, member model.ProjectMember) error {
	if member.UserID == actor.ID {
		return aitool.BadRequest("project_member.cannot_remove_self", "cannot remove the currently authenticated actor from the project")
	}
	if member.Role == authz.ProjectRoleOwner {
		actorIsOwner, err := h.projectMemberOwnerAllowedForAI(tc, project.ID, actor.ID)
		if err != nil {
			return err
		}
		if !actorIsOwner {
			return aitool.Forbidden("auth.forbidden", "only a real project owner member can remove an owner member")
		}
		if !h.projectHasAnotherOwner(tc.Context(), project.ID, member.ID) {
			return aitool.BadRequest("project_member.last_owner_protected", "at least one owner member must remain in the project")
		}
	}
	return nil
}

func (h *Handler) aiPreviewWorkspaceRemoveMember(tc *aitool.ToolContext, in workspaceRemoveMemberInput) (*aitool.PreviewResult, error) {
	actor, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	member, memberUser, err := resolveExistingProjectMemberForAI(db, project.ID, in.Member)
	if err != nil {
		return nil, err
	}
	if err := h.validateMemberRemovalForAI(tc, actor, project, member); err != nil {
		return nil, err
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Remove member %s (%s, role=%s) from project %q", memberUser.Name, memberUser.Email, member.Role, project.Name),
			TargetResource: "project_member",
			TargetID:       member.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "member", Before: fmt.Sprintf("%s (%s)", memberUser.Email, member.Role), Action: "delete"},
			},
		},
		LockedPayload: lockedProjectMutation{
			ProjectID:        project.ID,
			ExistingMemberID: member.ID,
		},
	}, nil
}

func (h *Handler) aiExecuteWorkspaceRemoveMember(tc *aitool.ToolContext, in workspaceRemoveMemberInput) (map[string]any, error) {
	actor, project, _, err := h.authorizeProjectForAI(tc, in.Project, authz.ActionProjectManage)
	if err != nil {
		return nil, err
	}
	if err := ensureProjectCanMutateForAI(project); err != nil {
		return nil, err
	}
	db := h.aiDB(tc)

	var locked lockedProjectMutation
	_ = tc.UnmarshalLockedPayload(&locked)

	var member model.ProjectMember
	if locked.ExistingMemberID != "" {
		if err := db.First(&member, "id = ? AND project_id = ?", locked.ExistingMemberID, project.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, aitool.NotFound("project_member.not_found", "project member was not found")
			}
			return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
	} else {
		resolved, _, resolveErr := resolveExistingProjectMemberForAI(db, project.ID, in.Member)
		if resolveErr != nil {
			return nil, resolveErr
		}
		member = resolved
	}

	if err := h.validateMemberRemovalForAI(tc, actor, project, member); err != nil {
		return nil, err
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&member).Error; err != nil {
			return err
		}
		return publishProjectMemberInbox(tc.Context(), tx, projectMemberInboxInput{
			Type: "project.member_removed", Priority: "high", Project: project, Actor: actor,
			RecipientUserID: member.UserID, MemberID: member.ID, Role: member.Role,
			DedupKey: "project-member-removed:" + member.ID,
		})
	}); err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "inbox.operation_failed", "project member removal failed")
	}

	h.auditWithContext(actor.ID, "project_member.delete", member.ID, true, member.Role, tc.Context())
	defaultInboxBroker.Notify(member.UserID, "")

	return map[string]any{
		"id":        member.ID,
		"userId":    member.UserID,
		"projectId": project.ID,
		"removed":   true,
		"deleted":   true,
	}, nil
}
