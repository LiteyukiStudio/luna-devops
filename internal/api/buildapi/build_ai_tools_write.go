package buildapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/buildenv"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var gitCommitHexPattern = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)

type buildEnvVarItem struct {
	Key   string `json:"key" desc:"必填，变量键名（^[A-Za-z_][A-Za-z0-9_]*$，禁止 LITEYUKI_* 保留前缀）"`
	Value string `json:"value" desc:"必填，变量值"`
}

type buildSecretActionItem struct {
	Key       string  `json:"key" desc:"必填，加密密钥键名（^[A-Za-z_][A-Za-z0-9_]*$，禁止 LITEYUKI_* 保留前缀）"`
	Operation *string `json:"operation,omitempty" enum:"set|clear" default:"set" desc:"操作类型：set（默认，写入非空密钥）或 clear（删除该加密密钥）"`
	Value     *string `json:"value,omitempty" sensitive:"true" desc:"密钥明文值（operation=set 时必填非空）"`
}

type buildSaveVariableSetInput struct {
	VariableSet        *string                 `json:"variableSet,omitempty" desc:"可选，省略为新建；传入构建变量集 ID（bvs_xxx）或可管理范围内唯一名称为增量更新"`
	Name               *string                 `json:"name,omitempty" desc:"新建必填；变量集名称"`
	Scope              *string                 `json:"scope,omitempty" enum:"user|project|global" desc:"可选，新建传非空 addProjects 默认 project 否则默认 user；更新省略时严格保留原 scope"`
	AddProjects        []string                `json:"addProjects,omitempty" desc:"可选，scope=project 时增量授权项目"`
	RemoveProjects     []string                `json:"removeProjects,omitempty" desc:"可选，scope=project 时增量移除项目"`
	Enabled            *bool                   `json:"enabled,omitempty" desc:"可选，新建默认 true"`
	SetVariables       []buildEnvVarItem       `json:"setVariables,omitempty" desc:"可选，新增或更新普通构建变量（禁 LITEYUKI_* 前缀）"`
	RemoveVariableKeys []string                `json:"removeVariableKeys,omitempty" desc:"可选，删除指定普通构建变量键名"`
	SecretActions      []buildSecretActionItem `json:"secretActions,omitempty" desc:"可选，单键写入或清除构建加密密钥（operation: set|clear，禁 LITEYUKI_* 前缀）"`
}

type buildDeleteVariableSetInput struct {
	VariableSet string `json:"variableSet" desc:"必填，待删除的构建变量集 ID（bvs_xxx）或可管理范围内唯一名称"`
}

type buildConfigureScopedEnvInput struct {
	Scope              string                  `json:"scope" enum:"application|deployment" desc:"必填，作用域：application（应用级）或 deployment（部署目标级）"`
	Project            string                  `json:"project" desc:"必填，项目 ID 或 identifier"`
	Application        string                  `json:"application" desc:"必填，应用 ID、identifier 或唯一名称"`
	Target             *string                 `json:"target,omitempty" desc:"scope=deployment 时必填；支持 targetId、stage 或唯一名称"`
	SetVariables       []buildEnvVarItem       `json:"setVariables,omitempty" desc:"可选，新增或更新普通构建变量（禁 LITEYUKI_* 前缀）"`
	RemoveVariableKeys []string                `json:"removeVariableKeys,omitempty" desc:"可选，删除指定普通构建变量键名"`
	SecretActions      []buildSecretActionItem `json:"secretActions,omitempty" desc:"可选，单键写入或清除构建加密密钥（operation: set|clear，禁 LITEYUKI_* 前缀）"`
}

type adminConfigureGlobalBuildEnvInput struct {
	SetVariables       []buildEnvVarItem       `json:"setVariables,omitempty" desc:"可选，新增或更新平台全局构建普通变量（禁 LITEYUKI_* 前缀）"`
	RemoveVariableKeys []string                `json:"removeVariableKeys,omitempty" desc:"可选，删除指定普通变量键名"`
	SecretActions      []buildSecretActionItem `json:"secretActions,omitempty" desc:"可选，单键写入或清除平台全局构建加密密钥（operation: set|clear，禁 LITEYUKI_* 前缀）"`
}

type buildTriggerRunInput struct {
	Project           string            `json:"project" desc:"必填，项目 ID 或 identifier"`
	Application       string            `json:"application" desc:"必填，应用 ID、identifier 或唯一名称"`
	Target            *string           `json:"target,omitempty" desc:"可选，支持 targetId、stage 或名称；省略时仅当应用下存在唯一 1 个活跃代码源部署目标时自动选中"`
	SourceBranch      *string           `json:"sourceBranch,omitempty" desc:"可选，构建分支（禁止以 - 开头；与 sourceTag 互斥，均省略时取绑定仓库默认分支）"`
	SourceTag         *string           `json:"sourceTag,omitempty" desc:"可选，构建 Git Tag（禁止以 - 开头；与 sourceBranch 互斥）"`
	SourceCommit      *string           `json:"sourceCommit,omitempty" pattern:"^[0-9a-fA-F]{7,64}$" desc:"可选，精确十六进制 Commit SHA（7-64 位）"`
	TargetTag         *string           `json:"targetTag,omitempty" desc:"可选，覆盖本次构建产物镜像 Tag"`
	OverrideBuildArgs map[string]string `json:"overrideBuildArgs,omitempty" desc:"可选，仅对本次构建在目标 buildArgs 基础上逐键合并覆盖的 Docker --build-arg（禁 LITEYUKI_* 前缀）"`
}

type buildRetryRunInput struct {
	Project    string `json:"project" desc:"必填，项目 ID 或 identifier"`
	BuildRunID string `json:"buildRunId" desc:"必填，历史构建记录 ID（bldr_xxx）"`
}

type buildCancelRunInput struct {
	Project    string `json:"project" desc:"必填，项目 ID 或 identifier"`
	BuildRunID string `json:"buildRunId" desc:"必填，待取消的构建记录 ID（bldr_xxx）"`
}

type buildDeleteRunInput struct {
	Project    string `json:"project" desc:"必填，项目 ID 或 identifier"`
	BuildRunID string `json:"buildRunId" desc:"必填，仅终态（succeeded/failed/canceled/timeout）构建记录可删除"`
}

type triggerRunPreviewLock struct {
	ResolvedTargetID string `json:"resolvedTargetId"`
}

func (h *Handler) registerBuildWriteAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[buildSaveVariableSetInput, aiBuildVariableSetOutput]{
		Name:           "build_save_variable_set",
		Domain:         aitool.DomainBuild,
		Title:          "创建或增量更新构建变量与密钥集",
		Summary:        "创建或纯增量修改构建变量与加密密钥集，省略字段与未提及密钥保持原状",
		Tags:           []string{"build", "variable", "secret", "env"},
		AliasesZH:      []string{"保存构建变量集", "配置构建密钥"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionBuildTrigger), string(authz.ActionSecretUpdate)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "build_list_variable_sets",
			Family:         "resource",
			Expect:         "present",
			IDFrom:         "result.id",
			ReadIDArgument: "variableSet",
			Collection:     true,
		}},
		Preview: h.aiPreviewSaveVariableSet,
		Execute: h.aiExecuteSaveVariableSet,
	})

	aitool.Register(reg, aitool.Spec[buildDeleteVariableSetInput, map[string]any]{
		Name:           "build_delete_variable_set",
		Domain:         aitool.DomainBuild,
		Title:          "删除构建变量与密钥集",
		Summary:        "删除构建变量与密钥集并自动从关联部署目标解绑",
		Tags:           []string{"build", "variable", "secret", "delete"},
		AliasesZH:      []string{"删除构建变量集", "移除构建密钥集"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionBuildDelete), string(authz.ActionSecretUpdate)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "build_list_variable_sets",
			Family:         "resource",
			Expect:         "absent",
			IDFrom:         "result.id",
			ReadIDArgument: "variableSet",
			Collection:     true,
		}},
		Preview: h.aiPreviewDeleteVariableSet,
		Execute: h.aiExecuteDeleteVariableSet,
	})

	aitool.Register(reg, aitool.Spec[buildConfigureScopedEnvInput, map[string]any]{
		Name:           "build_configure_scoped_env",
		Domain:         aitool.DomainBuild,
		Title:          "增量配置应用或部署环境专属构建变量",
		Summary:        "纯增量配置应用级或部署目标级专属构建期变量与加密密钥（仅影响镜像构建阶段，修改运行期环境变量请调 delivery_configure_target_config）",
		Tags:           []string{"build", "environment", "variable", "secret"},
		AliasesZH:      []string{"配置构建环境变量", "设置构建密钥"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionBuildTrigger), string(authz.ActionSecretViewValue), string(authz.ActionSecretUpdate)},
		Verification: []aitool.OperationVerification{{
			OperationID: "build_get_scoped_env",
			Family:      "resource",
			Expect:      "present",
			Singleton:   true,
		}},
		Preview: h.aiPreviewConfigureScopedEnv,
		Execute: h.aiExecuteConfigureScopedEnv,
	})

	aitool.Register(reg, aitool.Spec[buildTriggerRunInput, map[string]any]{
		Name:           "build_trigger_run",
		Domain:         aitool.DomainBuild,
		Title:          "触发镜像构建任务",
		Summary:        "异步触发一次基于当前最新配置的容器镜像构建任务",
		Tags:           []string{"build", "trigger", "run", "image"},
		AliasesZH:      []string{"触发构建", "开始构建镜像", "构建应用"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectAsync,
		RequiredScopes: []string{string(authz.ActionBuildTrigger)},
		Verification: []aitool.OperationVerification{{
			OperationID:     "build_get_run",
			Family:          "build_run",
			Expect:          "present",
			IDFrom:          "result.buildRunId",
			ReadIDArgument:  "buildRunId",
			StatusField:     "status",
			PendingStatuses: []string{"queued", "running"},
			SuccessStatuses: []string{"succeeded"},
			FailureStatuses: []string{"failed", "canceled", "timeout"},
		}},
		Preview: h.aiPreviewTriggerBuildRun,
		Execute: h.aiExecuteTriggerBuildRun,
	})

	aitool.Register(reg, aitool.Spec[buildRetryRunInput, map[string]any]{
		Name:    "build_retry_run",
		Domain:  aitool.DomainBuild,
		Title:   "基于历史快照重试构建任务",
		Summary: "基于历史构建当时的配置与变量快照重新排队构建（若刚修改过构建变量、密钥或流水线配置，请调用 build_trigger_run 而非重试）",
		AvoidWhen: aitool.Text(
			"重试会原样复用历史构建当时的全部配置与变量快照；若刚修改过构建变量、密钥或构建流水线配置，请勿调用 build_retry_run，必须调用 build_trigger_run 才会读取最新配置！",
			"Retrying reuses the historical configuration and variable snapshot; if you just updated build variables, secrets, or pipeline settings, call build_trigger_run instead.",
		),
		Tags:           []string{"build", "retry", "run"},
		AliasesZH:      []string{"重试构建", "重新构建历史任务"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectAsync,
		RequiredScopes: []string{string(authz.ActionBuildTrigger)},
		Verification: []aitool.OperationVerification{{
			OperationID:     "build_get_run",
			Family:          "build_run",
			Expect:          "present",
			IDFrom:          "result.buildRunId",
			ReadIDArgument:  "buildRunId",
			StatusField:     "status",
			PendingStatuses: []string{"queued", "running"},
			SuccessStatuses: []string{"succeeded"},
			FailureStatuses: []string{"failed", "canceled", "timeout"},
		}},
		Preview: h.aiPreviewRetryBuildRun,
		Execute: h.aiExecuteRetryBuildRun,
	})

	aitool.Register(reg, aitool.Spec[buildCancelRunInput, map[string]any]{
		Name:           "build_cancel_run",
		Domain:         aitool.DomainBuild,
		Title:          "取消排队或运行中的构建任务",
		Summary:        "终止排队中（queued）或运行中（running）的镜像构建任务",
		Tags:           []string{"build", "cancel", "stop", "run"},
		AliasesZH:      []string{"取消构建", "终止构建任务"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionBuildCancel)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "build_get_run",
			Family:         "build_run",
			Expect:         "canceled",
			IDFrom:         "result.buildRunId",
			ReadIDArgument: "buildRunId",
		}},
		Preview: h.aiPreviewCancelBuildRun,
		Execute: h.aiExecuteCancelBuildRun,
	})

	aitool.Register(reg, aitool.Spec[buildDeleteRunInput, map[string]any]{
		Name:           "build_delete_run",
		Domain:         aitool.DomainBuild,
		Title:          "删除已结束的构建记录及日志",
		Summary:        "删除已处于终态的构建记录及其构建日志与关联 Hook 日志",
		Tags:           []string{"build", "delete", "run", "log"},
		AliasesZH:      []string{"删除构建记录", "清理构建历史"},
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionBuildDelete)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "build_get_run",
			Family:         "build_run",
			Expect:         "absent",
			IDFrom:         "result.id",
			ReadIDArgument: "buildRunId",
		}},
		Preview: h.aiPreviewDeleteBuildRun,
		Execute: h.aiExecuteDeleteBuildRun,
	})
}

func (h *Handler) registerBuildAdminAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[adminGetGlobalBuildEnvInput, map[string]any]{
		Name:              "admin_get_global_build_env",
		Domain:            aitool.DomainAdmin,
		Title:             "读取平台全局共享构建环境变量",
		Summary:           "读取平台全局级（scope=global）共享构建环境变量与已配置加密密钥键名数组 secretKeys",
		Tags:              []string{"build", "environment", "global", "admin"},
		AliasesZH:         []string{"查看全局构建变量", "全局构建环境"},
		AdminOnly:         true,
		GlobalSessionOnly: true,
		ApprovalTier:      aitool.ApprovalTierL0,
		Effect:            aitool.EffectRead,
		RequiredScopes:    []string{string(authz.ActionBuildRead), string(authz.ActionSecretViewValue)},
		Execute:           h.aiGetGlobalBuildEnvAdmin,
	})

	aitool.Register(reg, aitool.Spec[adminConfigureGlobalBuildEnvInput, map[string]any]{
		Name:              "admin_configure_global_build_env",
		Domain:            aitool.DomainAdmin,
		Title:             "增量配置平台全局共享构建环境变量",
		Summary:           "纯增量配置平台全局级（scope=global）共享构建期变量与加密密钥",
		Tags:              []string{"build", "environment", "global", "admin", "secret"},
		AliasesZH:         []string{"配置全局构建变量", "设置全局构建密钥"},
		AdminOnly:         true,
		GlobalSessionOnly: true,
		ApprovalTier:      aitool.ApprovalTierL2,
		Effect:            aitool.EffectWrite,
		RequiredScopes:    []string{string(authz.ActionConfigWrite), string(authz.ActionSecretUpdate)},
		Verification: []aitool.OperationVerification{{
			OperationID: "admin_get_global_build_env",
			Family:      "resource",
			Expect:      "present",
			Singleton:   true,
		}},
		Preview: h.aiPreviewConfigureGlobalBuildEnvAdmin,
		Execute: h.aiExecuteConfigureGlobalBuildEnvAdmin,
	})
}

// --- Shared Variable / Secret Patch Engine ---

type envPatchPlan struct {
	finalVars     map[string]string
	secretsToSet  map[string]string
	secretsToKeep map[string]string
	secretsToDrop []string
	sensitiveKeys []string
}

func validateBuildVarKeyNotReserved(rawKey string) (string, error) {
	key := strings.TrimSpace(rawKey)
	if !isBuildEnvKey(key) {
		return "", aitool.BadRequest("argument_invalid", fmt.Sprintf("invalid variable/secret key %q: must match ^[A-Za-z_][A-Za-z0-9_]*$", rawKey))
	}
	if strings.HasPrefix(strings.ToUpper(key), "LITEYUKI_") {
		return "", aitool.BadRequest("build.reserved_variable_prefix", fmt.Sprintf("key %q uses reserved platform prefix LITEYUKI_*", key))
	}
	return key, nil
}

func computeBuildEnvPatch(
	existingVars map[string]string,
	existingSecretRefs map[string]string,
	setVars []buildEnvVarItem,
	removeVarKeys []string,
	secretActions []buildSecretActionItem,
) (envPatchPlan, error) {
	plan := envPatchPlan{
		finalVars:     make(map[string]string, len(existingVars)+len(setVars)),
		secretsToSet:  make(map[string]string),
		secretsToKeep: make(map[string]string),
	}
	for k, v := range existingVars {
		if isBuildEnvKey(k) {
			plan.finalVars[k] = v
		}
	}
	for k, ref := range existingSecretRefs {
		if isBuildEnvKey(k) && strings.TrimSpace(ref) != "" {
			plan.secretsToKeep[k] = ref
		}
	}

	for _, rk := range removeVarKeys {
		trimmed := strings.TrimSpace(rk)
		if trimmed != "" {
			delete(plan.finalVars, trimmed)
		}
	}

	for _, item := range setVars {
		key, err := validateBuildVarKeyNotReserved(item.Key)
		if err != nil {
			return plan, err
		}
		val := strings.TrimSpace(item.Value)
		if len(val) > 4096 {
			return plan, aitool.BadRequest("argument_invalid", fmt.Sprintf("variable %q exceeds maximum length 4096", key))
		}
		plan.finalVars[key] = val
	}

	seenSecretKeys := make(map[string]string, len(secretActions))
	for _, act := range secretActions {
		key, err := validateBuildVarKeyNotReserved(act.Key)
		if err != nil {
			return plan, err
		}
		op := strings.ToLower(fallback(buildTrimPtr(act.Operation), "set"))
		if prevOp, dup := seenSecretKeys[key]; dup && prevOp != op {
			return plan, aitool.BadRequest("argument_invalid", fmt.Sprintf("conflicting secretActions operations on key %q", key))
		}
		seenSecretKeys[key] = op

		switch op {
		case "set":
			val := buildTrimPtr(act.Value)
			if val == "" {
				return plan, aitool.BadRequest("argument_invalid", fmt.Sprintf("secretActions value for %q must be non-empty when operation=set", key))
			}
			if len(val) > 8192 {
				return plan, aitool.BadRequest("argument_invalid", fmt.Sprintf("secret value for %q exceeds maximum length 8192", key))
			}
			plan.secretsToSet[key] = val
			delete(plan.secretsToKeep, key)
			if !slices.Contains(plan.sensitiveKeys, key) {
				plan.sensitiveKeys = append(plan.sensitiveKeys, key)
			}
		case "clear":
			if buildTrimPtr(act.Value) != "" {
				return plan, aitool.BadRequest("argument_invalid", fmt.Sprintf("do not pass value when clearing secret %q", key))
			}
			delete(plan.secretsToSet, key)
			delete(plan.secretsToKeep, key)
			if !slices.Contains(plan.secretsToDrop, key) {
				plan.secretsToDrop = append(plan.secretsToDrop, key)
			}
			if !slices.Contains(plan.sensitiveKeys, key) {
				plan.sensitiveKeys = append(plan.sensitiveKeys, key)
			}
		default:
			return plan, aitool.BadRequest("argument_invalid", fmt.Sprintf("unsupported secretActions operation %q; expected set or clear", op))
		}
	}

	// Bidirectional key conflict check between plain variables and active secrets (spec lines 607, 620, 1524).
	for key := range plan.finalVars {
		_, inKeep := plan.secretsToKeep[key]
		_, inSet := plan.secretsToSet[key]
		if inKeep || inSet {
			return plan, aitool.NewErrorWithHint(
				http.StatusConflict,
				"build.variable_key_conflict",
				fmt.Sprintf("key %q cannot exist in both plain variables and encrypted secrets simultaneously", key),
				"Remove the key from removeVariableKeys or clear it via secretActions(operation=\"clear\") before switching its type.",
			)
		}
	}
	sort.Strings(plan.sensitiveKeys)
	return plan, nil
}

func enforceBuildScopedConversationSandbox(conversationProjectID, currentUserID, scope, ownerRef string, projectIDs []string) error {
	convProj := strings.TrimSpace(conversationProjectID)
	if convProj == "" {
		return nil
	}
	normScope := strings.ToLower(strings.TrimSpace(scope))
	if normScope == "user" && strings.TrimSpace(ownerRef) == currentUserID {
		return nil
	}
	cleaned := sortedProjectIDs(projectIDs)
	if normScope == "project" && len(cleaned) == 1 && cleaned[0] == convProj {
		return nil
	}
	err := aitool.Forbidden("ai.project_scope_mismatch", "single-project conversation may only mutate personal (scope=user) or single-project resources bound to the current project")
	err.Hint = "Switch to an unbound global conversation to manage global or multi-project shared resources."
	return err
}

func buildResolveProjectRefs(db *gorm.DB, refs []string) ([]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	resolved := make([]string, 0, len(refs))
	for _, ref := range refs {
		proj, err := aitool.ResolveProject(db, ref)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(resolved, proj.ID) {
			resolved = append(resolved, proj.ID)
		}
	}
	return sortedProjectIDs(resolved), nil
}

func buildMergeProjectIDs(existing, addIDs, removeIDs []string) []string {
	removeSet := make(map[string]bool, len(removeIDs))
	for _, id := range removeIDs {
		removeSet[strings.TrimSpace(id)] = true
	}
	merged := make([]string, 0, len(existing)+len(addIDs))
	for _, id := range existing {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" && !removeSet[trimmed] && !slices.Contains(merged, trimmed) {
			merged = append(merged, trimmed)
		}
	}
	for _, id := range addIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" && !removeSet[trimmed] && !slices.Contains(merged, trimmed) {
			merged = append(merged, trimmed)
		}
	}
	return sortedProjectIDs(merged)
}

func (h *Handler) canManageVariableSetSilent(tc *aitool.ToolContext, set model.BuildVariableSet) bool {
	child := aitool.NewToolContext(tc.ParentGin, tc.DB, tc.User, tc.SessionID, tc.ConversationID, tc.ConversationProjectID, tc.RunID, tc.ToolCallID, tc.IdempotencyKey, tc.InputMode, nil)
	h.syncGinUser(child)
	return h.canManageScopedResourceByID(child.Gin, tc.User, set.Scope, set.OwnerRef, scopedResourceBuildVariableSet, set.ID, "forbidden")
}

func (h *Handler) aiResolveManageableVariableSet(tc *aitool.ToolContext, ref string) (model.BuildVariableSet, error) {
	var all []model.BuildVariableSet
	if err := tc.DB.Order("created_at asc").Find(&all).Error; err != nil {
		return model.BuildVariableSet{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	manageable := make([]model.BuildVariableSet, 0, len(all))
	for _, s := range all {
		if h.canManageVariableSetSilent(tc, s) {
			manageable = append(manageable, s)
		}
	}
	resolved, err := aitool.ResolveUniqueByIDOrName(
		manageable,
		"build_variable_set",
		"build.variable_set_not_found",
		ref,
		func(s model.BuildVariableSet) string { return s.ID },
		func(s model.BuildVariableSet) string { return s.Name },
	)
	if err != nil {
		return model.BuildVariableSet{}, err
	}
	resolved.ProjectIDs = h.scopedResourceProjectIDs(scopedResourceBuildVariableSet, resolved.ID, tc.Context())
	return resolved, nil
}

// --- 25. build_save_variable_set ---

type preparedVariableSetSave struct {
	isCreate bool
	existing model.BuildVariableSet
	target   model.BuildVariableSet
	patch    envPatchPlan
}

func (h *Handler) prepareSaveVariableSet(tc *aitool.ToolContext, in buildSaveVariableSetInput) (preparedVariableSetSave, error) {
	h.syncGinUser(tc)
	var prep preparedVariableSetSave

	addIDs, err := buildResolveProjectRefs(tc.DB, in.AddProjects)
	if err != nil {
		return prep, err
	}
	removeIDs, err := buildResolveProjectRefs(tc.DB, in.RemoveProjects)
	if err != nil {
		return prep, err
	}

	setRef := buildTrimPtr(in.VariableSet)
	if setRef == "" {
		prep.isCreate = true
		name := buildTrimPtr(in.Name)
		if name == "" {
			return prep, aitool.BadRequest("argument_invalid", "name is required when creating a BuildVariableSet")
		}
		rawScope := buildTrimPtr(in.Scope)
		if rawScope == "" {
			if len(addIDs) > 0 {
				rawScope = "project"
			} else {
				rawScope = "user"
			}
		}
		projectIDs := buildMergeProjectIDs(nil, addIDs, removeIDs)
		scope, ownerRef, normalizedProjects, ok := h.normalizeScopedOwnerWithProjects(
			tc.Gin, tc.User, rawScope, "", projectIDs, "platform administrator role is required for global build variable sets",
		)
		if !ok {
			return prep, tc.AbortedError("build.variable_set_scope_invalid", "invalid build variable set scope")
		}
		if err := enforceBuildScopedConversationSandbox(tc.ConversationProjectID, tc.User.ID, scope, ownerRef, normalizedProjects); err != nil {
			return prep, err
		}
		patch, patchErr := computeBuildEnvPatch(nil, nil, in.SetVariables, in.RemoveVariableKeys, in.SecretActions)
		if patchErr != nil {
			return prep, patchErr
		}
		if len(patch.finalVars) == 0 && len(patch.secretsToSet) == 0 {
			return prep, aitool.BadRequest("argument_invalid", "at least one build variable or secret must be configured")
		}
		prep.target = model.BuildVariableSet{
			ID:         id.New("bvs"),
			Name:       name,
			Scope:      scope,
			OwnerRef:   ownerRef,
			ProjectIDs: normalizedProjects,
			Variables:  buildenv.Encode(patch.finalVars),
			Enabled:    buildBoolPtrDefault(in.Enabled, true),
			CreatedBy:  tc.User.ID,
		}
		prep.patch = patch
		return prep, nil
	}

	existing, err := h.aiResolveManageableVariableSet(tc, setRef)
	if err != nil {
		return prep, err
	}
	if err := enforceBuildScopedConversationSandbox(tc.ConversationProjectID, tc.User.ID, existing.Scope, existing.OwnerRef, existing.ProjectIDs); err != nil {
		return prep, err
	}

	next := existing
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" {
			return prep, aitool.BadRequest("argument_invalid", "name cannot be empty")
		}
		next.Name = n
	}
	if in.Enabled != nil {
		next.Enabled = *in.Enabled
	}

	scopeChanged := in.Scope != nil || len(addIDs) > 0 || len(removeIDs) > 0
	if scopeChanged {
		targetScope := existing.Scope
		if in.Scope != nil && strings.TrimSpace(*in.Scope) != "" {
			targetScope = strings.ToLower(strings.TrimSpace(*in.Scope))
		}
		mergedProjects := buildMergeProjectIDs(existing.ProjectIDs, addIDs, removeIDs)
		scope, ownerRef, normalizedProjects, ok := h.normalizeScopedOwnerWithProjects(
			tc.Gin, tc.User, targetScope, existing.OwnerRef, mergedProjects, "platform administrator role is required for global build variable sets",
		)
		if !ok {
			return prep, tc.AbortedError("build.variable_set_scope_invalid", "invalid build variable set scope")
		}
		next.Scope = scope
		next.OwnerRef = ownerRef
		next.ProjectIDs = normalizedProjects
	}
	if err := enforceBuildScopedConversationSandbox(tc.ConversationProjectID, tc.User.ID, next.Scope, next.OwnerRef, next.ProjectIDs); err != nil {
		return prep, err
	}

	patch, patchErr := computeBuildEnvPatch(
		buildenv.Decode(existing.Variables),
		decodeSecretRefs(existing.SecretRefs),
		in.SetVariables,
		in.RemoveVariableKeys,
		in.SecretActions,
	)
	if patchErr != nil {
		return prep, patchErr
	}
	if len(patch.finalVars) == 0 && len(patch.secretsToKeep) == 0 && len(patch.secretsToSet) == 0 {
		return prep, aitool.BadRequest("argument_invalid", "at least one build variable or secret must remain configured")
	}
	next.Variables = buildenv.Encode(patch.finalVars)

	prep.existing = existing
	prep.target = next
	prep.patch = patch
	return prep, nil
}

func (h *Handler) aiPreviewSaveVariableSet(tc *aitool.ToolContext, in buildSaveVariableSetInput) (*aitool.PreviewResult, error) {
	prep, err := h.prepareSaveVariableSet(tc, in)
	if err != nil {
		return nil, err
	}
	summary := fmt.Sprintf("Create %s build variable set %q", prep.target.Scope, prep.target.Name)
	targetID := ""
	var changes []aitool.PreviewChange
	if !prep.isCreate {
		targetID = prep.existing.ID
		summary = fmt.Sprintf("Update build variable set %q (%s)", prep.target.Name, prep.existing.ID)
		if prep.existing.Name != prep.target.Name {
			changes = append(changes, aitool.PreviewChange{Field: "name", Before: prep.existing.Name, After: prep.target.Name, Action: "update"})
		}
		if prep.existing.Scope != prep.target.Scope {
			changes = append(changes, aitool.PreviewChange{Field: "scope", Before: prep.existing.Scope, After: prep.target.Scope, Action: "update"})
		}
		if prep.existing.Enabled != prep.target.Enabled {
			changes = append(changes, aitool.PreviewChange{Field: "enabled", Before: prep.existing.Enabled, After: prep.target.Enabled, Action: "update"})
		}
	} else {
		changes = append(changes,
			aitool.PreviewChange{Field: "name", After: prep.target.Name, Action: "create"},
			aitool.PreviewChange{Field: "scope", After: prep.target.Scope, Action: "create"},
		)
	}
	for _, item := range in.SetVariables {
		changes = append(changes, aitool.PreviewChange{Field: "variables." + strings.TrimSpace(item.Key), After: item.Value, Action: "set"})
	}
	for _, rk := range in.RemoveVariableKeys {
		changes = append(changes, aitool.PreviewChange{Field: "variables." + strings.TrimSpace(rk), Action: "remove"})
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        summary,
			TargetResource: "build_variable_set",
			TargetID:       targetID,
			ProjectID:      strings.TrimSpace(tc.ConversationProjectID),
			Changes:        changes,
			SensitiveKeys:  prep.patch.sensitiveKeys,
		},
	}, nil
}

func (h *Handler) aiExecuteSaveVariableSet(tc *aitool.ToolContext, in buildSaveVariableSetInput) (aiBuildVariableSetOutput, error) {
	prep, err := h.prepareSaveVariableSet(tc, in)
	if err != nil {
		return aiBuildVariableSetOutput{}, err
	}
	if prep.isCreate {
		set := prep.target
		txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
			secretRefs := make(map[string]string, len(prep.patch.secretsToSet))
			for key, val := range prep.patch.secretsToSet {
				if h.secretAvailable() {
					ref, storeErr := h.storeSecretWithDB(tc.Context(), tx, val, tc.User.ID, "build_variable_set:"+set.ID+":"+key)
					if storeErr != nil {
						return storeErr
					}
					secretRefs[key] = ref
				} else {
					secretRefs[key] = "inline-unset"
				}
			}
			set.SecretRefs = buildenv.Encode(secretRefs)
			if err := tx.Create(&set).Error; err != nil {
				return err
			}
			return h.replaceScopedResourceProjectBindings(tx, scopedResourceBuildVariableSet, set.ID, sortedProjectIDs(set.ProjectIDs), nil)
		})
		if txErr != nil {
			return aiBuildVariableSetOutput{}, aitool.BadRequest("build.variable_set_save_failed", txErr.Error())
		}
		h.auditWithContext(tc.User.ID, "build_variable_set.create", set.ID, true, set.Scope, tc.Context())
		return h.formatAIBuildVariableSet(tc.User, set, tc.Context())
	}

	var updated model.BuildVariableSet
	txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
		var locked model.BuildVariableSet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", prep.existing.ID).Error; err != nil {
			return err
		}
		oldRefs := decodeSecretRefs(locked.SecretRefs)
		nextRefs := make(map[string]string, len(prep.patch.secretsToKeep)+len(prep.patch.secretsToSet))
		for key := range prep.patch.secretsToKeep {
			if existingRef := strings.TrimSpace(oldRefs[key]); existingRef != "" {
				nextRefs[key] = existingRef
			}
		}
		for key, val := range prep.patch.secretsToSet {
			if h.secretAvailable() {
				res := "build_variable_set:" + locked.ID + ":" + key
				newRef, storeErr := h.storeSecretWithDB(tc.Context(), tx, val, tc.User.ID, res)
				if storeErr != nil {
					return storeErr
				}
				nextRefs[key] = newRef
			} else {
				nextRefs[key] = "inline-unset"
			}
		}
		if h.secretAvailable() {
			for key, oldRef := range oldRefs {
				if strings.TrimSpace(oldRef) == "" || nextRefs[key] == oldRef {
					continue
				}
				if err := h.deleteSecretRefWithDB(tc.Context(), tx, oldRef, "build_variable_set:"+locked.ID+":"+key); err != nil {
					return err
				}
			}
		}
		locked.Name = prep.target.Name
		locked.Scope = prep.target.Scope
		locked.OwnerRef = prep.target.OwnerRef
		locked.ProjectIDs = prep.target.ProjectIDs
		locked.Variables = prep.target.Variables
		locked.SecretRefs = buildenv.Encode(nextRefs)
		locked.Enabled = prep.target.Enabled
		if err := tx.Save(&locked).Error; err != nil {
			return err
		}
		projectIDs := sortedProjectIDs(locked.ProjectIDs)
		if err := h.replaceScopedResourceProjectBindings(tx, scopedResourceBuildVariableSet, locked.ID, projectIDs, nil); err != nil {
			return err
		}
		if err := pruneInaccessibleBuildVariableSetTargets(tx, locked, projectIDs); err != nil {
			return err
		}
		updated = locked
		return nil
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return aiBuildVariableSetOutput{}, aitool.NotFound("build.variable_set_not_found", "build variable set not found")
	}
	if txErr != nil {
		return aiBuildVariableSetOutput{}, aitool.BadRequest("build.variable_set_save_failed", txErr.Error())
	}
	h.auditWithContext(tc.User.ID, "build_variable_set.update", updated.ID, true, updated.Scope, tc.Context())
	return h.formatAIBuildVariableSet(tc.User, updated, tc.Context())
}

// --- 26. build_delete_variable_set ---

func (h *Handler) checkDeleteVariableSet(tc *aitool.ToolContext, in buildDeleteVariableSetInput) (model.BuildVariableSet, []string, error) {
	h.syncGinUser(tc)
	set, err := h.aiResolveManageableVariableSet(tc, in.VariableSet)
	if err != nil {
		return model.BuildVariableSet{}, nil, err
	}
	if err := enforceBuildScopedConversationSandbox(tc.ConversationProjectID, tc.User.ID, set.Scope, set.OwnerRef, set.ProjectIDs); err != nil {
		return model.BuildVariableSet{}, nil, err
	}
	var targets []model.DeploymentTarget
	if err := tc.DB.Select("id", "name", "stage", "build_variable_set_ids").
		Where("deleted_at IS NULL").
		Find(&targets).Error; err != nil {
		return model.BuildVariableSet{}, nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	var impacted []string
	for _, t := range targets {
		if slices.Contains(buildVariableSetIDs(t.BuildVariableSetIDs), set.ID) {
			impacted = append(impacted, fmt.Sprintf("%s (%s)", t.Name, t.Stage))
		}
	}
	return set, impacted, nil
}

func (h *Handler) aiPreviewDeleteVariableSet(tc *aitool.ToolContext, in buildDeleteVariableSetInput) (*aitool.PreviewResult, error) {
	set, impacted, err := h.checkDeleteVariableSet(tc, in)
	if err != nil {
		return nil, err
	}
	var impactLines []string
	if len(impacted) > 0 {
		impactLines = append(impactLines, fmt.Sprintf("Will automatically detach from %d deployment target(s): %s", len(impacted), strings.Join(impacted, ", ")))
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Delete build variable set %q (%s)", set.Name, set.ID),
			TargetResource: "build_variable_set",
			TargetID:       set.ID,
			ProjectID:      strings.TrimSpace(tc.ConversationProjectID),
			Changes: []aitool.PreviewChange{
				{Field: "variableSet", Before: set.Name, Action: "delete"},
			},
			Impact: impactLines,
		},
	}, nil
}

func (h *Handler) aiExecuteDeleteVariableSet(tc *aitool.ToolContext, in buildDeleteVariableSetInput) (map[string]any, error) {
	set, _, err := h.checkDeleteVariableSet(tc, in)
	if err != nil {
		return nil, err
	}
	txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
		var locked model.BuildVariableSet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", set.ID).Error; err != nil {
			return err
		}
		var targets []model.DeploymentTarget
		if err := tx.Select("id", "build_variable_set_ids").Find(&targets).Error; err != nil {
			return err
		}
		for _, t := range targets {
			nextIDs := removeBuildVariableSetID(t.BuildVariableSetIDs, locked.ID)
			if nextIDs != t.BuildVariableSetIDs {
				if err := tx.Model(&model.DeploymentTarget{}).Where("id = ?", t.ID).Update("build_variable_set_ids", nextIDs).Error; err != nil {
					return err
				}
			}
		}
		if h.secretAvailable() {
			for key, ref := range decodeSecretRefs(locked.SecretRefs) {
				if strings.TrimSpace(ref) == "" {
					continue
				}
				if err := h.deleteSecretRefWithDB(tc.Context(), tx, ref, "build_variable_set:"+locked.ID+":"+key); err != nil {
					return err
				}
			}
		}
		if err := tx.Where("resource_type = ? AND resource_id = ?", scopedResourceBuildVariableSet, locked.ID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
			return err
		}
		return tx.Delete(&locked).Error
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return nil, aitool.NotFound("build.variable_set_not_found", "build variable set not found")
	}
	if txErr != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "build.variable_set_delete_failed", txErr.Error())
	}
	h.auditWithContext(tc.User.ID, "build_variable_set.delete", set.ID, true, set.Scope, tc.Context())
	out := map[string]any{
		"id":      set.ID,
		"deleted": true,
	}
	if strings.TrimSpace(tc.ConversationProjectID) != "" {
		out["projectId"] = strings.TrimSpace(tc.ConversationProjectID)
	}
	return out, nil
}

// --- 27 & 33. build_configure_scoped_env & admin_configure_global_build_env ---

func (h *Handler) applyBuildEnvironmentPatchTx(
	tc *aitool.ToolContext,
	scope, scopeRef string,
	setVars []buildEnvVarItem,
	removeVarKeys []string,
	secretActions []buildSecretActionItem,
	dryRun bool,
) (model.BuildEnvironmentConfig, envPatchPlan, error) {
	if len(setVars) == 0 && len(removeVarKeys) == 0 && len(secretActions) == 0 {
		return model.BuildEnvironmentConfig{}, envPatchPlan{}, aitool.BadRequest("argument_invalid", "at least one of setVariables, removeVariableKeys, or secretActions must be non-empty")
	}
	existing, findErr := h.findBuildEnvironmentConfig(tc.DB, scope, scopeRef)
	if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return model.BuildEnvironmentConfig{}, envPatchPlan{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", findErr.Error())
	}
	patch, patchErr := computeBuildEnvPatch(
		buildenv.Decode(existing.Variables),
		decodeSecretRefs(existing.SecretRefs),
		setVars,
		removeVarKeys,
		secretActions,
	)
	if patchErr != nil {
		return model.BuildEnvironmentConfig{}, envPatchPlan{}, patchErr
	}
	if dryRun {
		return existing, patch, nil
	}

	var saved model.BuildEnvironmentConfig
	txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
		var locked model.BuildEnvironmentConfig
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "scope = ? AND scope_ref = ?", scope, scopeRef).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		isNew := errors.Is(err, gorm.ErrRecordNotFound)
		if isNew {
			locked = model.BuildEnvironmentConfig{
				ID:         id.New("bec"),
				Scope:      scope,
				ScopeRef:   scopeRef,
				Variables:  "{}",
				SecretRefs: "{}",
			}
		}

		// Recompute against locked row inside transaction for strict isolation.
		lockedPatch, reErr := computeBuildEnvPatch(
			buildenv.Decode(locked.Variables),
			decodeSecretRefs(locked.SecretRefs),
			setVars,
			removeVarKeys,
			secretActions,
		)
		if reErr != nil {
			return reErr
		}

		oldRefs := decodeSecretRefs(locked.SecretRefs)
		nextRefs := make(map[string]string, len(lockedPatch.secretsToKeep)+len(lockedPatch.secretsToSet))
		for key := range lockedPatch.secretsToKeep {
			if existingRef := strings.TrimSpace(oldRefs[key]); existingRef != "" {
				nextRefs[key] = existingRef
			}
		}
		for key, val := range lockedPatch.secretsToSet {
			if h.secretAvailable() {
				res := "build_environment:" + scope + ":" + scopeRef + ":" + key
				newRef, storeErr := h.storeSecretWithDB(tc.Context(), tx, val, tc.User.ID, res)
				if storeErr != nil {
					return storeErr
				}
				nextRefs[key] = newRef
			} else {
				nextRefs[key] = "inline-unset"
			}
		}
		if h.secretAvailable() {
			for key, oldRef := range oldRefs {
				if strings.TrimSpace(oldRef) == "" || nextRefs[key] == oldRef {
					continue
				}
				if delErr := h.deleteSecretRefWithDB(tc.Context(), tx, oldRef, "build_environment:"+scope+":"+scopeRef+":"+key); delErr != nil {
					return delErr
				}
			}
		}

		locked.Variables = buildenv.Encode(lockedPatch.finalVars)
		locked.SecretRefs = buildenv.Encode(nextRefs)
		locked.UpdatedBy = tc.User.ID
		if saveErr := tx.Save(&locked).Error; saveErr != nil {
			return saveErr
		}
		saved = locked
		patch = lockedPatch
		return nil
	})
	if txErr != nil {
		if te, ok := aitool.AsToolError(txErr); ok {
			return model.BuildEnvironmentConfig{}, envPatchPlan{}, te
		}
		return model.BuildEnvironmentConfig{}, envPatchPlan{}, aitool.NewError(http.StatusInternalServerError, "build.environment_save_failed", txErr.Error())
	}
	return saved, patch, nil
}

func (h *Handler) aiPreviewConfigureScopedEnv(tc *aitool.ToolContext, in buildConfigureScopedEnvInput) (*aitool.PreviewResult, error) {
	project, app, target, scope, scopeRef, err := h.resolveScopedBuildEnvTarget(tc, in.Scope, in.Project, in.Application, in.Target)
	if err != nil {
		return nil, err
	}
	if !h.ensureProjectCanMutate(tc.Gin, project) {
		return nil, tc.AbortedError("project.delete_in_progress", "project cannot be mutated")
	}
	_, patch, patchErr := h.applyBuildEnvironmentPatchTx(tc, scope, scopeRef, in.SetVariables, in.RemoveVariableKeys, in.SecretActions, true)
	if patchErr != nil {
		return nil, patchErr
	}
	summary := fmt.Sprintf("Update %s build environment variables on application %s", scope, app.Name)
	if target != nil {
		summary = fmt.Sprintf("Update deployment build environment variables on %s/%s", app.Name, target.Stage)
	}
	var changes []aitool.PreviewChange
	for _, item := range in.SetVariables {
		changes = append(changes, aitool.PreviewChange{Field: "variables." + strings.TrimSpace(item.Key), After: item.Value, Action: "set"})
	}
	for _, rk := range in.RemoveVariableKeys {
		changes = append(changes, aitool.PreviewChange{Field: "variables." + strings.TrimSpace(rk), Action: "remove"})
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        summary,
			TargetResource: "build_environment_config",
			TargetID:       scopeRef,
			ProjectID:      project.ID,
			Changes:        changes,
			SensitiveKeys:  patch.sensitiveKeys,
		},
	}, nil
}

func (h *Handler) aiExecuteConfigureScopedEnv(tc *aitool.ToolContext, in buildConfigureScopedEnvInput) (map[string]any, error) {
	project, app, target, scope, scopeRef, err := h.resolveScopedBuildEnvTarget(tc, in.Scope, in.Project, in.Application, in.Target)
	if err != nil {
		return nil, err
	}
	if !h.ensureProjectCanMutate(tc.Gin, project) {
		return nil, tc.AbortedError("project.delete_in_progress", "project cannot be mutated")
	}
	saved, _, saveErr := h.applyBuildEnvironmentPatchTx(tc, scope, scopeRef, in.SetVariables, in.RemoveVariableKeys, in.SecretActions, false)
	if saveErr != nil {
		return nil, saveErr
	}
	h.auditWithContext(tc.User.ID, "build_environment.update", scope+":"+scopeRef, true, project.ID, tc.Context())
	out := formatBuildEnvConfigOutput(saved)
	out["projectId"] = project.ID
	out["applicationId"] = app.ID
	out["requiresRelease"] = false
	if target != nil {
		out["deploymentTargetId"] = target.ID
		out["stage"] = target.Stage
	}
	return out, nil
}

func (h *Handler) aiPreviewConfigureGlobalBuildEnvAdmin(tc *aitool.ToolContext, in adminConfigureGlobalBuildEnvInput) (*aitool.PreviewResult, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("auth.platform_admin_required", "platform administrator role is required")
	}
	_, patch, err := h.applyBuildEnvironmentPatchTx(
		tc, model.BuildEnvironmentScopeGlobal, model.BuildEnvironmentGlobalRef,
		in.SetVariables, in.RemoveVariableKeys, in.SecretActions, true,
	)
	if err != nil {
		return nil, err
	}
	var changes []aitool.PreviewChange
	for _, item := range in.SetVariables {
		changes = append(changes, aitool.PreviewChange{Field: "variables." + strings.TrimSpace(item.Key), After: item.Value, Action: "set"})
	}
	for _, rk := range in.RemoveVariableKeys {
		changes = append(changes, aitool.PreviewChange{Field: "variables." + strings.TrimSpace(rk), Action: "remove"})
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        "Update global platform build environment variables and secrets",
			TargetResource: "build_environment_config",
			TargetID:       model.BuildEnvironmentGlobalRef,
			Changes:        changes,
			SensitiveKeys:  patch.sensitiveKeys,
		},
	}, nil
}

func (h *Handler) aiExecuteConfigureGlobalBuildEnvAdmin(tc *aitool.ToolContext, in adminConfigureGlobalBuildEnvInput) (map[string]any, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("auth.platform_admin_required", "platform administrator role is required")
	}
	saved, _, err := h.applyBuildEnvironmentPatchTx(
		tc, model.BuildEnvironmentScopeGlobal, model.BuildEnvironmentGlobalRef,
		in.SetVariables, in.RemoveVariableKeys, in.SecretActions, false,
	)
	if err != nil {
		return nil, err
	}
	h.auditWithContext(tc.User.ID, "build_environment.update", "global:platform", true, "", tc.Context())
	return formatBuildEnvConfigOutput(saved), nil
}

// --- 28. build_trigger_run ---

func (h *Handler) artifactRegistryVisibleForProject(db *gorm.DB, user model.User, registry model.ArtifactRegistry, projectID string) bool {
	switch strings.ToLower(strings.TrimSpace(registry.Scope)) {
	case "global", "":
		return true
	case "user":
		return strings.TrimSpace(registry.OwnerRef) == user.ID
	case "project":
		var count int64
		_ = db.Model(&model.ScopedResourceProjectBinding{}).
			Where("resource_type = ? AND resource_id = ? AND project_id = ?", "artifact_registry", registry.ID, projectID).
			Count(&count).Error
		return count > 0
	default:
		return false
	}
}

func (h *Handler) prepareTriggerBuildRun(tc *aitool.ToolContext, in buildTriggerRunInput, lockedTargetID string) (model.Project, model.Application, model.DeploymentTarget, model.BuildRun, bool, error) {
	h.syncGinUser(tc)
	user, project, ok := h.authorizeProjectRef(tc.Gin, in.Project, authz.ActionBuildTrigger)
	if !ok {
		return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, tc.AbortedError("project.forbidden", "project is not accessible")
	}
	if !h.ensureProjectCanMutate(tc.Gin, project) {
		return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, tc.AbortedError("project.delete_in_progress", "project cannot be mutated")
	}
	app, err := aitool.ResolveApplication(tc.DB, project.ID, in.Application)
	if err != nil {
		return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, err
	}
	if !h.applicationCanMutate(app) {
		return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, aitool.Conflict("application.delete_in_progress", "application is being deleted")
	}

	var target model.DeploymentTarget
	targetRef := buildTrimPtr(in.Target)
	if lockedTargetID != "" {
		targetRef = lockedTargetID
	}
	if targetRef != "" {
		resolved, tErr := aitool.ResolveDeploymentTarget(tc.DB, project.ID, app.ID, targetRef)
		if tErr != nil {
			return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, tErr
		}
		if !slices.Contains(model.ActiveDeleteStatuses, resolved.DeleteStatus) && resolved.DeleteStatus != "" {
			return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, aitool.Conflict("deployment_target.delete_in_progress", "deployment target is being deleted")
		}
		if !resolved.Enabled {
			return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, aitool.Conflict("deployment_target.disabled", fmt.Sprintf("deployment target %s (%s) is disabled", resolved.Name, resolved.Stage))
		}
		if h.normalizeDeploymentSourceType(resolved.SourceType) != "repository" {
			return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, aitool.BadRequest("build.source_type_not_repository", "image-sourced deployment target cannot trigger source builds")
		}
		target = resolved
	} else {
		var activeRepoTargets []model.DeploymentTarget
		if err := tc.DB.Where(
			"project_id = ? AND application_id = ? AND enabled = ? AND delete_status IN ? AND (source_type = ? OR source_type = '') AND deleted_at IS NULL",
			project.ID, app.ID, true, model.ActiveDeleteStatuses, "repository",
		).Order("created_at asc").Find(&activeRepoTargets).Error; err != nil {
			return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		if len(activeRepoTargets) == 0 {
			var disabledCount int64
			_ = tc.DB.Model(&model.DeploymentTarget{}).
				Where("project_id = ? AND application_id = ? AND enabled = ? AND deleted_at IS NULL", project.ID, app.ID, false).
				Count(&disabledCount).Error
			if disabledCount > 0 {
				return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, aitool.Conflict("deployment_target.disabled", "all deployment targets on this application are disabled")
			}
			return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, aitool.NotFound("deployment_target.not_found", "no active repository-sourced deployment target was found")
		}
		if len(activeRepoTargets) > 1 {
			candidates := make([]aitool.AmbiguousCandidate, len(activeRepoTargets))
			for i, t := range activeRepoTargets {
				candidates[i] = aitool.AmbiguousCandidate{ID: t.ID, Name: t.Name, Stage: t.Stage}
			}
			return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, aitool.AmbiguousResource("deployment_target", "omitted target", candidates)
		}
		target = activeRepoTargets[0]
	}

	sourceBranch := buildTrimPtr(in.SourceBranch)
	sourceTag := buildTrimPtr(in.SourceTag)
	if sourceBranch != "" && sourceTag != "" {
		return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, aitool.BadRequest("argument_invalid", "sourceBranch and sourceTag are mutually exclusive")
	}
	if strings.HasPrefix(sourceBranch, "-") || strings.HasPrefix(sourceTag, "-") {
		return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, aitool.BadRequest("argument_invalid", "sourceBranch and sourceTag must not start with '-'")
	}
	sourceCommit := buildTrimPtr(in.SourceCommit)
	if sourceCommit != "" && !gitCommitHexPattern.MatchString(sourceCommit) {
		return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, aitool.BadRequest("argument_invalid", "sourceCommit must be a 7..64 character hexadecimal Git SHA")
	}

	if sourceBranch == "" && sourceTag == "" && strings.TrimSpace(target.RepositoryBindingID) != "" {
		var binding model.RepositoryBinding
		if err := tc.DB.First(&binding, "id = ? AND project_id = ?", target.RepositoryBindingID, project.ID).Error; err == nil {
			sourceBranch = fallback(strings.TrimSpace(binding.DefaultBranch), "main")
		}
	}

	mergedArgs := model.BuildArgs(target.BuildArgs)
	if mergedArgs == nil {
		mergedArgs = map[string]string{}
	}
	for rawKey, rawVal := range in.OverrideBuildArgs {
		key := strings.TrimSpace(rawKey)
		if !model.IsBuildArgKey(key) {
			return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, aitool.BadRequest("deployment.build_args_invalid", fmt.Sprintf("invalid overrideBuildArgs key %q", rawKey))
		}
		if strings.HasPrefix(strings.ToUpper(key), "LITEYUKI_") {
			return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, aitool.BadRequest("build.reserved_variable_prefix", fmt.Sprintf("overrideBuildArgs key %q uses reserved LITEYUKI_* prefix", key))
		}
		val := strings.TrimSpace(rawVal)
		if len(val) > 4096 {
			return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, aitool.BadRequest("deployment.build_args_invalid", fmt.Sprintf("overrideBuildArgs value for %q is too long", key))
		}
		mergedArgs[key] = val
	}

	if strings.TrimSpace(target.TargetRegistryID) != "" {
		var reg model.ArtifactRegistry
		if err := tc.DB.First(&reg, "id = ?", target.TargetRegistryID).Error; err != nil {
			return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, aitool.NotFound("registry.not_found", "target artifact registry was not found")
		}
		if !h.artifactRegistryVisibleForProject(tc.DB, user, reg, project.ID) {
			return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, aitool.Forbidden("registry.forbidden", "target artifact registry is not accessible in this project")
		}
	}

	if !h.ensureBillingAllowsNewBuild(tc.Gin, project.ID) {
		return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, tc.AbortedError("billing.insufficient_balance", "billing check rejected new build")
	}

	explicitTargetTag := in.TargetTag != nil && strings.TrimSpace(*in.TargetTag) != ""
	run := model.BuildRun{
		ID:                 id.New("bldr"),
		ProjectID:          project.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		Status:             "queued",
		TriggerType:        "manual",
		SourceBranch:       sourceBranch,
		SourceTag:          sourceTag,
		SourceCommit:       sourceCommit,
		BuildArgs:          model.EncodeBuildArgs(mergedArgs),
		TargetRegistryID:   strings.TrimSpace(target.TargetRegistryID),
		TargetTag:          buildTrimPtr(in.TargetTag),
		CreatedBy:          user.ID,
		TriggeredByName:    buildRunActorName(user),
		TriggeredByEmail:   strings.TrimSpace(user.Email),
	}
	if err := h.prepareBuildRunRequestWithOptions(user, &run, explicitTargetTag, tc.Context()); err != nil {
		return model.Project{}, model.Application{}, model.DeploymentTarget{}, model.BuildRun{}, false, buildRunValidationToolError(err)
	}
	return project, app, target, run, explicitTargetTag, nil
}

func (h *Handler) aiPreviewTriggerBuildRun(tc *aitool.ToolContext, in buildTriggerRunInput) (*aitool.PreviewResult, error) {
	project, app, target, run, _, err := h.prepareTriggerBuildRun(tc, in, "")
	if err != nil {
		return nil, err
	}
	ref := run.SourceBranch
	if run.SourceTag != "" {
		ref = "tag:" + run.SourceTag
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Trigger image build on %s/%s (%s -> %s)", app.Name, target.Stage, ref, run.ImageRef),
			TargetResource: "deployment_target",
			TargetID:       target.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "stage", After: target.Stage, Action: "trigger"},
				{Field: "sourceRef", After: ref, Action: "trigger"},
				{Field: "targetImage", After: run.ImageRef, Action: "trigger"},
			},
		},
		LockedPayload: triggerRunPreviewLock{ResolvedTargetID: target.ID},
	}, nil
}

func (h *Handler) aiExecuteTriggerBuildRun(tc *aitool.ToolContext, in buildTriggerRunInput) (map[string]any, error) {
	var locked triggerRunPreviewLock
	_ = tc.UnmarshalLockedPayload(&locked)

	h.syncGinUser(tc)
	idempotencyKey, validKey := transport.DeliveryIdempotencyKey(tc.Gin, "build.trigger")
	if !validKey {
		return nil, aitool.BadRequest("request.idempotency_key_invalid", "Idempotency-Key must contain 8 to 128 characters")
	}

	project, app, target, run, _, err := h.prepareTriggerBuildRun(tc, in, locked.ResolvedTargetID)
	if err != nil {
		return nil, err
	}

	requestHash := transport.DeliveryRequestHash(map[string]any{
		"projectId":         project.ID,
		"applicationId":     app.ID,
		"targetId":          target.ID,
		"sourceBranch":      run.SourceBranch,
		"sourceTag":         run.SourceTag,
		"sourceCommit":      run.SourceCommit,
		"targetTag":         run.TargetTag,
		"overrideBuildArgs": in.OverrideBuildArgs,
	})
	if idempotencyKey != "" {
		existing, found, available := h.findBuildRunByIdempotency(tc.Gin, tc.User.ID, project.ID, idempotencyKey, requestHash)
		if !available {
			return nil, tc.AbortedError("request.idempotency_conflict", "idempotency check failed")
		}
		if found {
			return queuedBuildRunOutput(existing, project.ID, target.Stage, ""), nil
		}
	}
	run.IdempotencyKey = idempotencyKey
	run.IdempotencyRequestHash = requestHash

	queuedRun, qErr := h.queueBuildRun(tc.Context(), tc.User, run)
	if qErr != nil {
		if idempotencyKey != "" && !errors.Is(qErr, errBuildQueueUnavailable) && !strings.Contains(qErr.Error(), "投递失败") {
			existing, found, available := h.findBuildRunByIdempotency(tc.Gin, tc.User.ID, project.ID, idempotencyKey, requestHash)
			if !available {
				return nil, tc.AbortedError("request.idempotency_conflict", "idempotency check failed")
			}
			if found {
				return queuedBuildRunOutput(existing, project.ID, target.Stage, ""), nil
			}
		}
		if errors.Is(qErr, errBuildQueueUnavailable) || strings.Contains(qErr.Error(), "投递失败") {
			return nil, aitool.NewError(http.StatusServiceUnavailable, "build.queue_unavailable", qErr.Error())
		}
		return nil, aitool.BadRequest("build.trigger_failed", qErr.Error())
	}

	h.auditWithContext(tc.User.ID, "build_run.trigger", queuedRun.ID, true, queuedRun.ImageRef, tc.Context())
	return queuedBuildRunOutput(queuedRun, project.ID, target.Stage, ""), nil
}

// --- 29. build_retry_run ---

func (h *Handler) verifyRetryVariableSetsSecurity(db *gorm.DB, actor model.User, projectID string, previous model.BuildRun) error {
	unionSetIDs := make([]string, 0)
	addSetID := func(id string) {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" && !slices.Contains(unionSetIDs, trimmed) {
			unionSetIDs = append(unionSetIDs, trimmed)
		}
	}
	for _, setID := range buildVariableSetIDs(previous.BuildVariableSetIDs) {
		addSetID(setID)
	}

	var secretValueIDs []string
	for _, ref := range buildenv.Decode(previous.BuildSecretRefsSnapshot) {
		if secretID, ok := strings.CutPrefix(strings.TrimSpace(ref), "secret-id:"); ok && strings.TrimSpace(secretID) != "" {
			secretValueIDs = append(secretValueIDs, strings.TrimSpace(secretID))
		}
	}
	if len(secretValueIDs) > 0 {
		var secretRows []model.SecretValue
		if err := db.Select("id", "resource").Where("id IN ?", secretValueIDs).Find(&secretRows).Error; err != nil {
			return aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		for _, row := range secretRows {
			if rest, ok := strings.CutPrefix(strings.TrimSpace(row.Resource), "build_variable_set:"); ok {
				if setID, _, hasKey := strings.Cut(rest, ":"); hasKey {
					addSetID(setID)
				}
			}
		}
	}

	for _, setID := range unionSetIDs {
		var set model.BuildVariableSet
		if err := db.Unscoped().First(&set, "id = ?", setID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
		}
		switch strings.ToLower(strings.TrimSpace(set.Scope)) {
		case "user":
			if strings.TrimSpace(set.OwnerRef) != actor.ID {
				return aitool.Forbidden("build.variable_set_forbidden", "cannot retry a build run that captured another user's personal build variable set")
			}
		case "project":
			if !set.DeletedAt.Valid {
				var boundCount int64
				if err := db.Model(&model.ScopedResourceProjectBinding{}).
					Where("resource_type = ? AND resource_id = ? AND project_id = ?", scopedResourceBuildVariableSet, set.ID, projectID).
					Count(&boundCount).Error; err != nil {
					return aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
				}
				if boundCount == 0 {
					return aitool.Forbidden("build.variable_set_project_revoked", fmt.Sprintf("shared build variable set %s is no longer authorized for this project", set.ID))
				}
			}
		}
	}
	return nil
}

func (h *Handler) prepareRetryBuildRun(tc *aitool.ToolContext, in buildRetryRunInput) (model.Project, model.DeploymentTarget, model.BuildRun, model.BuildRun, error) {
	h.syncGinUser(tc)
	user, project, ok := h.authorizeProjectRef(tc.Gin, in.Project, authz.ActionBuildTrigger)
	if !ok {
		return model.Project{}, model.DeploymentTarget{}, model.BuildRun{}, model.BuildRun{}, tc.AbortedError("project.forbidden", "project is not accessible")
	}
	if !h.ensureProjectCanMutate(tc.Gin, project) {
		return model.Project{}, model.DeploymentTarget{}, model.BuildRun{}, model.BuildRun{}, tc.AbortedError("project.delete_in_progress", "project cannot be mutated")
	}
	var previous model.BuildRun
	if err := tc.DB.First(&previous, "id = ? AND project_id = ?", strings.TrimSpace(in.BuildRunID), project.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Project{}, model.DeploymentTarget{}, model.BuildRun{}, model.BuildRun{}, aitool.NotFound("build_run.not_found", "historical build run was not found")
		}
		return model.Project{}, model.DeploymentTarget{}, model.BuildRun{}, model.BuildRun{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}

	var target model.DeploymentTarget
	if err := tc.DB.First(&target, "id = ? AND project_id = ? AND deleted_at IS NULL", previous.DeploymentTargetID, project.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Project{}, model.DeploymentTarget{}, model.BuildRun{}, model.BuildRun{}, aitool.NotFound("deployment_target.not_found", "deployment target for build run was not found")
		}
		return model.Project{}, model.DeploymentTarget{}, model.BuildRun{}, model.BuildRun{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if !target.Enabled {
		return model.Project{}, model.DeploymentTarget{}, model.BuildRun{}, model.BuildRun{}, aitool.Conflict("deployment_target.disabled", fmt.Sprintf("deployment target %s (%s) is disabled", target.Name, target.Stage))
	}

	if strings.TrimSpace(previous.TargetRegistryID) != "" {
		var reg model.ArtifactRegistry
		if err := tc.DB.First(&reg, "id = ?", previous.TargetRegistryID).Error; err != nil {
			return model.Project{}, model.DeploymentTarget{}, model.BuildRun{}, model.BuildRun{}, aitool.NotFound("registry.not_found", "historical target artifact registry was not found")
		}
		if !h.artifactRegistryVisibleForProject(tc.DB, user, reg, project.ID) {
			return model.Project{}, model.DeploymentTarget{}, model.BuildRun{}, model.BuildRun{}, aitool.Forbidden("registry.forbidden", "historical target artifact registry is no longer accessible in this project")
		}
	}

	if secErr := h.verifyRetryVariableSetsSecurity(tc.DB, user, project.ID, previous); secErr != nil {
		return model.Project{}, model.DeploymentTarget{}, model.BuildRun{}, model.BuildRun{}, secErr
	}

	if !h.ensureBillingAllowsNewBuild(tc.Gin, project.ID) {
		return model.Project{}, model.DeploymentTarget{}, model.BuildRun{}, model.BuildRun{}, tc.AbortedError("billing.insufficient_balance", "billing check rejected retry build")
	}

	run := model.BuildRun{
		ID:                      id.New("bldr"),
		ProjectID:               previous.ProjectID,
		ApplicationID:           previous.ApplicationID,
		DeploymentTargetID:      previous.DeploymentTargetID,
		BuildVariableSetIDs:     previous.BuildVariableSetIDs,
		BuildVariablesSnapshot:  previous.BuildVariablesSnapshot,
		BuildSecretRefsSnapshot: previous.BuildSecretRefsSnapshot,
		Status:                  "queued",
		TriggerType:             "retry",
		SourceBranch:            previous.SourceBranch,
		SourceTag:               previous.SourceTag,
		SourceCommit:            previous.SourceCommit,
		BuildDefinitionMode:     previous.BuildDefinitionMode,
		BuildTemplateID:         previous.BuildTemplateID,
		BuildTemplateVersion:    previous.BuildTemplateVersion,
		BuildTemplateValues:     previous.BuildTemplateValues,
		BuildTemplateDockerfile: previous.BuildTemplateDockerfile,
		BuildTemplateChecksum:   previous.BuildTemplateChecksum,
		DockerfilePath:          previous.DockerfilePath,
		BuildContext:            previous.BuildContext,
		BuildDirectory:          previous.BuildDirectory,
		BuildArgs:               previous.BuildArgs,
		BuildEnvironmentID:      previous.BuildEnvironmentID,
		BuildCPURequest:         previous.BuildCPURequest,
		BuildMemoryRequest:      previous.BuildMemoryRequest,
		BuildTimeoutSeconds:     previous.BuildTimeoutSeconds,
		TargetRegistryID:        previous.TargetRegistryID,
		TargetRepository:        previous.TargetRepository,
		TargetTag:               previous.TargetTag,
		CreatedBy:               user.ID,
		TriggeredByName:         buildRunActorName(user),
		TriggeredByEmail:        strings.TrimSpace(user.Email),
		SourceAuthorName:        previous.SourceAuthorName,
		SourceAuthorEmail:       previous.SourceAuthorEmail,
		SourceCommitMessage:     previous.SourceCommitMessage,
	}
	if err := h.prepareBuildRunRequestWithOptions(user, &run, strings.TrimSpace(previous.TargetTag) != "", tc.Context()); err != nil {
		return model.Project{}, model.DeploymentTarget{}, model.BuildRun{}, model.BuildRun{}, buildRunValidationToolError(err)
	}
	return project, target, previous, run, nil
}

func (h *Handler) aiPreviewRetryBuildRun(tc *aitool.ToolContext, in buildRetryRunInput) (*aitool.PreviewResult, error) {
	project, target, previous, run, err := h.prepareRetryBuildRun(tc, in)
	if err != nil {
		return nil, err
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Retry historical build run %s on stage %s (-> %s)", previous.ID, target.Stage, run.ImageRef),
			TargetResource: "build_run",
			TargetID:       previous.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "retriedFromBuildRunId", After: previous.ID, Action: "retry"},
				{Field: "targetImage", After: run.ImageRef, Action: "retry"},
			},
			Warnings: []string{
				"Retrying reuses the historical configuration and variable snapshot from the original build run.",
			},
		},
	}, nil
}

func (h *Handler) aiExecuteRetryBuildRun(tc *aitool.ToolContext, in buildRetryRunInput) (map[string]any, error) {
	h.syncGinUser(tc)
	idempotencyKey, validKey := transport.DeliveryIdempotencyKey(tc.Gin, "build.retry")
	if !validKey {
		return nil, aitool.BadRequest("request.idempotency_key_invalid", "Idempotency-Key must contain 8 to 128 characters")
	}

	project, target, previous, run, err := h.prepareRetryBuildRun(tc, in)
	if err != nil {
		return nil, err
	}

	requestHash := transport.DeliveryRequestHash(map[string]string{
		"projectId": project.ID,
		"runId":     previous.ID,
	})
	if idempotencyKey != "" {
		existing, found, available := h.findBuildRunByIdempotency(tc.Gin, tc.User.ID, project.ID, idempotencyKey, requestHash)
		if !available {
			return nil, tc.AbortedError("request.idempotency_conflict", "idempotency check failed")
		}
		if found {
			return queuedBuildRunOutput(existing, project.ID, target.Stage, previous.ID), nil
		}
	}
	run.IdempotencyKey = idempotencyKey
	run.IdempotencyRequestHash = requestHash

	queuedRun, qErr := h.queueBuildRun(tc.Context(), tc.User, run)
	if qErr != nil {
		if idempotencyKey != "" && !errors.Is(qErr, errBuildQueueUnavailable) && !strings.Contains(qErr.Error(), "投递失败") {
			existing, found, available := h.findBuildRunByIdempotency(tc.Gin, tc.User.ID, project.ID, idempotencyKey, requestHash)
			if !available {
				return nil, tc.AbortedError("request.idempotency_conflict", "idempotency check failed")
			}
			if found {
				return queuedBuildRunOutput(existing, project.ID, target.Stage, previous.ID), nil
			}
		}
		if errors.Is(qErr, errBuildQueueUnavailable) || strings.Contains(qErr.Error(), "投递失败") {
			return nil, aitool.NewError(http.StatusServiceUnavailable, "build.queue_unavailable", qErr.Error())
		}
		return nil, aitool.BadRequest("build.retry_failed", qErr.Error())
	}

	h.auditWithContext(tc.User.ID, "build_run.retry", queuedRun.ID, true, previous.ID, tc.Context())
	return queuedBuildRunOutput(queuedRun, project.ID, target.Stage, previous.ID), nil
}

// --- 30. build_cancel_run ---

func (h *Handler) checkCancelBuildRun(tc *aitool.ToolContext, in buildCancelRunInput) (model.Project, model.BuildRun, error) {
	h.syncGinUser(tc)
	_, project, ok := h.authorizeProjectRef(tc.Gin, in.Project, authz.ActionBuildCancel)
	if !ok {
		return model.Project{}, model.BuildRun{}, tc.AbortedError("project.forbidden", "project is not accessible")
	}
	var run model.BuildRun
	if err := tc.DB.First(&run, "id = ? AND project_id = ?", strings.TrimSpace(in.BuildRunID), project.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Project{}, model.BuildRun{}, aitool.NotFound("build_run.not_found", "build run was not found")
		}
		return model.Project{}, model.BuildRun{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if run.Status == "canceled" || buildRunCancelable(run.Status) {
		return project, run, nil
	}
	return model.Project{}, model.BuildRun{}, aitool.Conflict("build.not_cancelable", fmt.Sprintf("build run %s is in terminal status %q and cannot be canceled", run.ID, run.Status))
}

func (h *Handler) aiPreviewCancelBuildRun(tc *aitool.ToolContext, in buildCancelRunInput) (*aitool.PreviewResult, error) {
	project, run, err := h.checkCancelBuildRun(tc, in)
	if err != nil {
		return nil, err
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Cancel build run %s (current status: %s)", run.ID, run.Status),
			TargetResource: "build_run",
			TargetID:       run.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "status", Before: run.Status, After: "canceled", Action: "cancel"},
			},
		},
	}, nil
}

func (h *Handler) aiExecuteCancelBuildRun(tc *aitool.ToolContext, in buildCancelRunInput) (map[string]any, error) {
	project, run, err := h.checkCancelBuildRun(tc, in)
	if err != nil {
		return nil, err
	}
	if run.Status == "canceled" {
		out := buildRunToMap(run)
		out["projectId"] = project.ID
		out["buildRunId"] = run.ID
		return out, nil
	}

	finishedAt := time.Now()
	var alreadyCanceled bool
	txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
		var locked model.BuildRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ? AND project_id = ?", run.ID, project.ID).Error; err != nil {
			return err
		}
		if locked.Status == "canceled" {
			alreadyCanceled = true
			return nil
		}
		if !buildRunCancelable(locked.Status) {
			return errBuildRunNotCancelable
		}
		if err := tx.Model(&model.BuildJob{}).
			Where("build_run_id = ? AND project_id = ? AND status IN ?", locked.ID, locked.ProjectID, model.BuildActiveStatuses).
			Updates(map[string]any{
				"status":       "canceled",
				"failure_code": model.BuildFailureCanceledByUser,
				"finished_at":  &finishedAt,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&model.BuildRun{}).
			Where("id = ? AND status IN ?", locked.ID, model.BuildActiveStatuses).
			Updates(map[string]any{
				"status":       "canceled",
				"failure_code": model.BuildFailureCanceledByUser,
				"finished_at":  &finishedAt,
			}).Error
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return nil, aitool.NotFound("build_run.not_found", "build run was not found")
	}
	if errors.Is(txErr, errBuildRunNotCancelable) {
		return nil, aitool.Conflict("build.not_cancelable", "only queued or running build runs can be canceled")
	}
	if txErr != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "build.cancel_failed", txErr.Error())
	}
	if !alreadyCanceled {
		h.auditWithContext(tc.User.ID, "build_run.cancel", run.ID, true, "", tc.Context())
	}
	var refreshed model.BuildRun
	if err := tc.DB.First(&refreshed, "id = ? AND project_id = ?", run.ID, project.ID).Error; err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	out := buildRunToMap(refreshed)
	out["projectId"] = project.ID
	out["buildRunId"] = refreshed.ID
	return out, nil
}

// --- 31. build_delete_run ---

func (h *Handler) checkDeleteBuildRun(tc *aitool.ToolContext, in buildDeleteRunInput) (model.Project, model.BuildRun, error) {
	h.syncGinUser(tc)
	_, project, ok := h.authorizeProjectRef(tc.Gin, in.Project, authz.ActionBuildDelete)
	if !ok {
		return model.Project{}, model.BuildRun{}, tc.AbortedError("project.forbidden", "project is not accessible")
	}
	var run model.BuildRun
	if err := tc.DB.First(&run, "id = ? AND project_id = ?", strings.TrimSpace(in.BuildRunID), project.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Project{}, model.BuildRun{}, aitool.NotFound("build_run.not_found", "build run was not found")
		}
		return model.Project{}, model.BuildRun{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	if !buildRunTerminal(run.Status) {
		return model.Project{}, model.BuildRun{}, aitool.NewErrorWithHint(
			http.StatusConflict,
			"build.not_deletable",
			fmt.Sprintf("build run %s is still %q; only terminal build runs can be deleted", run.ID, run.Status),
			"Call build_cancel_run first if you want to stop an active build run before deleting it.",
		)
	}
	return project, run, nil
}

func (h *Handler) aiPreviewDeleteBuildRun(tc *aitool.ToolContext, in buildDeleteRunInput) (*aitool.PreviewResult, error) {
	project, run, err := h.checkDeleteBuildRun(tc, in)
	if err != nil {
		return nil, err
	}
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Delete terminal build run %s (%s) and its build/hook logs", run.ID, run.Status),
			TargetResource: "build_run",
			TargetID:       run.ID,
			ProjectID:      project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "buildRun", Before: run.ID, Action: "delete"},
			},
		},
	}, nil
}

func (h *Handler) aiExecuteDeleteBuildRun(tc *aitool.ToolContext, in buildDeleteRunInput) (map[string]any, error) {
	project, run, err := h.checkDeleteBuildRun(tc, in)
	if err != nil {
		return nil, err
	}
	txErr := tc.DB.Transaction(func(tx *gorm.DB) error {
		var locked model.BuildRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ? AND project_id = ?", run.ID, project.ID).Error; err != nil {
			return err
		}
		if !buildRunTerminal(locked.Status) {
			return errBuildRunNotCancelable
		}
		var hookRuns []model.HookRun
		if err := tx.Where("build_run_id = ? AND project_id = ?", locked.ID, locked.ProjectID).Find(&hookRuns).Error; err != nil {
			return err
		}
		if len(hookRuns) > 0 {
			hookRunIDs := make([]string, len(hookRuns))
			for i, hr := range hookRuns {
				hookRunIDs[i] = hr.ID
			}
			if err := tx.Where("hook_run_id IN ? AND project_id = ?", hookRunIDs, locked.ProjectID).Delete(&model.HookRunLog{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ? AND project_id = ?", hookRunIDs, locked.ProjectID).Delete(&model.HookRun{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("build_run_id = ? AND project_id = ?", locked.ID, locked.ProjectID).Delete(&model.BuildLog{}).Error; err != nil {
			return err
		}
		if err := tx.Where("build_run_id = ? AND project_id = ?", locked.ID, locked.ProjectID).Delete(&model.BuildJob{}).Error; err != nil {
			return err
		}
		return tx.Delete(&locked).Error
	})
	if errors.Is(txErr, gorm.ErrRecordNotFound) {
		return nil, aitool.NotFound("build_run.not_found", "build run was not found")
	}
	if errors.Is(txErr, errBuildRunNotCancelable) {
		return nil, aitool.Conflict("build.not_deletable", "only terminal build runs can be deleted")
	}
	if txErr != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "build.delete_failed", txErr.Error())
	}
	h.auditWithContext(tc.User.ID, "build_run.delete", run.ID, true, "", tc.Context())
	return map[string]any{
		"id":         run.ID,
		"buildRunId": run.ID,
		"projectId":  project.ID,
		"deleted":    true,
	}, nil
}

func buildRunValidationToolError(err error) *aitool.ToolError {
	var reqErr buildRunRequestError
	if errors.As(err, &reqErr) {
		code := reqErr.code
		if code == "" {
			if reqErr.status == http.StatusConflict {
				code = "build.conflict"
			} else {
				code = "build.request_invalid"
			}
		}
		return aitool.NewError(reqErr.status, code, reqErr.message)
	}
	return aitool.BadRequest("build.request_invalid", err.Error())
}

func queuedBuildRunOutput(run model.BuildRun, projectID, stage, retriedFrom string) map[string]any {
	out := buildRunToMap(run)
	out["id"] = run.ID
	out["buildRunId"] = run.ID
	out["projectId"] = projectID
	if stage != "" {
		out["stage"] = stage
	}
	if retriedFrom != "" {
		out["retriedFromBuildRunId"] = retriedFrom
	}
	return out
}

var (
	_ = context.Background
	_ = json.Marshal
)
