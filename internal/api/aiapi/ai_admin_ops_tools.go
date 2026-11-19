package aiapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/agentobservability"
	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aimodel"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/fixeddecimal"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

type opsWebSearchInput struct {
	Query string `json:"query" required:"true" min:"1" max:"300" desc:"搜索关键词（1..300 字符；自动拦截含凭据 URL 或敏感词查询）"`
	Limit *int   `json:"limit,omitempty" min:"1" max:"10" default:"5" desc:"返回搜索结果条数上限（默认 5，范围 1..10）"`
}

type opsWebFetchInput struct {
	URL           string `json:"url" required:"true" min:"1" max:"2048" format:"uri" desc:"待抓取的公开 http(s):// URL（禁止内网、元数据地址与内嵌凭据）"`
	MaxCharacters *int   `json:"maxCharacters,omitempty" min:"1" max:"50000" default:"20000" desc:"按 Unicode 字符数截断正文长度（默认 20000，上限 50000）"`
}

type adminListAIModelsInput struct{}

type adminInspectAgentObservabilityInput struct {
	View           string  `json:"view" required:"true" enum:"overview|conversations|turns|tools|tool_calls|conversation_detail|trace_detail" desc:"AI 可观测性查询视图"`
	Range          *string `json:"range,omitempty" enum:"1h|6h|24h|7d|30d|1y" default:"1h" desc:"时间范围窗口（默认 1h）"`
	Search         *string `json:"search,omitempty" desc:"按关键词过滤对话、Turn 或工具摘要"`
	OperationID    *string `json:"operationId,omitempty" desc:"view=tool_calls 时必填，指定工具 operationId"`
	ConversationID *string `json:"conversationId,omitempty" desc:"view=conversation_detail 时必填，指定对话 ID"`
	TraceID        *string `json:"traceId,omitempty" desc:"view=trace_detail 时必填，指定 Trace ID"`
	SortBy         *string `json:"sortBy,omitempty" desc:"列表排序字段（如 createdAt / durationMs / totalTokens / callCount / errorRate）"`
	SortOrder      *string `json:"sortOrder,omitempty" enum:"asc|desc" default:"desc" desc:"排序方向（默认 desc）"`
	Page           *int    `json:"page,omitempty" min:"1" default:"1" desc:"页码（>=1）"`
	PageSize       *int    `json:"pageSize,omitempty" min:"1" max:"50" default:"20" desc:"每页条数（默认 20，上限 50）"`
}

type adminSaveAIModelInput struct {
	Model                        *string `json:"model,omitempty" desc:"省略为新增 AI 模型；传入模型 ID（aimod_xxx）或唯一 name 为存量基底增量更新"`
	Name                         *string `json:"name,omitempty" desc:"模型唯一标识（新增必填，即传给上游 AI Provider 的模型名称）"`
	Enabled                      *bool   `json:"enabled,omitempty" desc:"是否启用模型（新增默认 true；禁止禁用最后一个启用模型）"`
	MaxContextTokens             *int64  `json:"maxContextTokens,omitempty" min:"4096" max:"2097152" desc:"上下文最大 Token 数（新增必填，4096..2097152）"`
	MaxOutputTokens              *int64  `json:"maxOutputTokens,omitempty" min:"256" max:"262144" desc:"单次输出最大 Token 数（新增必填，256..262144 且严格小于 maxContextTokens）"`
	InputCreditsPerMillion       *string `json:"inputCreditsPerMillion,omitempty" desc:"每百万输入 Token 积分单价（新增必填，非负十进制字符串）"`
	OutputCreditsPerMillion      *string `json:"outputCreditsPerMillion,omitempty" desc:"每百万输出 Token 积分单价（新增必填，非负十进制字符串）"`
	CachedInputCreditsPerMillion *string `json:"cachedInputCreditsPerMillion,omitempty" desc:"每百万缓存命中输入 Token 积分单价（新增必填，非负十进制字符串）"`
}

type adminDeleteAIModelInput struct {
	Model string `json:"model" required:"true" min:"1" desc:"待删除的 AI 模型 ID（aimod_xxx）或唯一 name（最后一个启用模型受保护不可删）"`
}

type adminTestAIIntegrationInput struct {
	Target   string  `json:"target" required:"true" enum:"ai_provider|observability_source" desc:"连通性探测目标类型"`
	Source   *string `json:"source,omitempty" enum:"prometheus|loki|tempo" desc:"target=observability_source 时必填的数据源类型"`
	URL      *string `json:"url,omitempty" desc:"target=observability_source 时可选临时探测地址；省略时回退读取系统已存配置"`
	Token    *string `json:"token,omitempty" sensitive:"true" desc:"target=observability_source 时可选临时访问令牌；省略时回退读取系统已存令牌"`
	TenantID *string `json:"tenantId,omitempty" desc:"target=observability_source 时可选多租户 X-Scope-OrgID；省略时回退读取系统已存配置"`
}

type aiModelTargetLock struct {
	ModelID string `json:"modelId"`
}

func (h *Handler) RegisterAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[opsWebSearchInput, map[string]any]{
		Name:           "ops_web_search",
		Domain:         aitool.DomainOps,
		Title:          "受控互联网技术检索",
		Summary:        "通过平台受控 SSRF 防护与出站代理搜索互联网公开技术资料（返回结果带 untrusted_external 标记）",
		AliasesZH:      []string{"联网搜索", "搜索互联网", "技术资料查询", "Web 搜索"},
		AliasesEN:      []string{"web search", "internet search"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{"web:read"},
		Execute:        h.executeOpsWebSearch,
	})

	aitool.Register(reg, aitool.Spec[opsWebFetchInput, map[string]any]{
		Name:           "ops_web_fetch",
		Domain:         aitool.DomainOps,
		Title:          "受控抓取公开网页正文",
		Summary:        "通过平台受控 SSRF 防护代理抓取公开网页可读正文并按字符数截断（返回结果带 untrusted_external 标记）",
		AliasesZH:      []string{"抓取网页", "读取网页", "打开链接", "Web Fetch"},
		AliasesEN:      []string{"fetch web page", "read url", "web fetch"},
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{"web:read"},
		Execute:        h.executeOpsWebFetch,
	})

	aitool.Register(reg, aitool.Spec[adminListAIModelsInput, map[string]any]{
		Name:           "admin_list_ai_models",
		Domain:         aitool.DomainAdmin,
		Title:          "查看平台 AI 模型目录",
		Summary:        "查看平台已配置的全部 AI 模型列表（含启停状态、上下文/输出 Token 上限与百万 Token 费率，不回显 API Key）",
		AliasesZH:      []string{"AI 模型列表", "查看大模型配置", "模型费率"},
		AliasesEN:      []string{"list ai models", "model catalog"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionConfigRead)},
		Execute:        h.executeAdminListAIModels,
	})

	aitool.Register(reg, aitool.Spec[adminInspectAgentObservabilityInput, map[string]any]{
		Name:           "admin_inspect_agent_observability",
		Domain:         aitool.DomainAdmin,
		Title:          "检查 AI Agent 运行可观测性",
		Summary:        "查询 AI Agent 可观测性概览、对话/Turn/工具调用统计或单条对话/Trace 链路详情",
		AliasesZH:      []string{"Agent 可观测性", "对话统计", "工具调用统计", "Trace 链路详情"},
		AliasesEN:      []string{"inspect agent observability", "agent metrics", "agent trace"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL0,
		Effect:         aitool.EffectRead,
		RequiredScopes: []string{string(authz.ActionAgentObservabilityRead)},
		Execute:        h.executeAdminInspectAgentObservability,
	})

	aitool.Register(reg, aitool.Spec[adminSaveAIModelInput, map[string]any]{
		Name:           "admin_save_ai_model",
		Domain:         aitool.DomainAdmin,
		Title:          "新增或增量更新 AI 模型配置",
		Summary:        "新增平台 AI 模型或基于存量记录增量修改上下文上限、输出上限、百万 Token 费率与启用状态",
		AliasesZH:      []string{"新增 AI 模型", "修改 AI 模型", "调整模型单价", "启停模型"},
		AliasesEN:      []string{"save ai model", "create ai model", "update ai model"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionConfigWrite)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "admin_list_ai_models",
			Family:         "resource",
			IDFrom:         "result.id",
			Collection:     true,
			CollectionPath: "items",
		}},
		Preview: h.previewAdminSaveAIModel,
		Execute: h.executeAdminSaveAIModel,
	})

	aitool.Register(reg, aitool.Spec[adminDeleteAIModelInput, map[string]any]{
		Name:           "admin_delete_ai_model",
		Domain:         aitool.DomainAdmin,
		Title:          "删除 AI 模型配置",
		Summary:        "删除指定的平台 AI 模型配置（最后一个启用中的模型受保护不可删除）",
		AliasesZH:      []string{"删除 AI 模型", "移除模型"},
		AliasesEN:      []string{"delete ai model", "remove ai model"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectWrite,
		RequiredScopes: []string{string(authz.ActionConfigWrite)},
		Verification: []aitool.OperationVerification{{
			OperationID:    "admin_list_ai_models",
			Family:         "resource",
			Expect:         "absent",
			IDFrom:         "result.id",
			Collection:     true,
			CollectionPath: "items",
		}},
		Preview: h.previewAdminDeleteAIModel,
		Execute: h.executeAdminDeleteAIModel,
	})

	aitool.Register(reg, aitool.Spec[adminTestAIIntegrationInput, map[string]any]{
		Name:           "admin_test_ai_integration",
		Domain:         aitool.DomainAdmin,
		Title:          "探测 AI Provider 或可观测数据源连通性",
		Summary:        "测试当前平台 AI Provider 连通性，或测试 Prometheus / Loki / Tempo 可观测数据源连接",
		AliasesZH:      []string{"测试 AI Provider", "测试模型连通性", "测试可观测数据源", "测试 Prometheus"},
		AliasesEN:      []string{"test ai provider", "test observability source"},
		AdminOnly:      true,
		ApprovalTier:   aitool.ApprovalTierL2,
		Effect:         aitool.EffectExecute,
		RequiredScopes: []string{string(authz.ActionConfigRead), string(authz.ActionAgentObservabilityRead)},
		Preview:        h.previewAdminTestAIIntegration,
		Execute:        h.executeAdminTestAIIntegration,
	})
}

func (h *Handler) executeOpsWebSearch(tc *aitool.ToolContext, in opsWebSearchInput) (map[string]any, error) {
	tc.SetNoStore()
	tools := h.host.AIToolService()
	if tools == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "ai.tool_service_unavailable", "AI web tool service is unavailable")
	}

	limit := 5
	if in.Limit != nil {
		limit = *in.Limit
	}
	args := map[string]any{
		"query": in.Query,
		"limit": limit,
	}
	res, err := tools.Execute(tc.Context(), aitool.Request{
		OperationID: "webSearch",
		UserID:      tc.User.ID,
		Arguments:   args,
	})
	if err != nil {
		return nil, mapWebToolError(err)
	}
	return normalizeWebToolOutput(res), nil
}

func (h *Handler) executeOpsWebFetch(tc *aitool.ToolContext, in opsWebFetchInput) (map[string]any, error) {
	tc.SetNoStore()
	tools := h.host.AIToolService()
	if tools == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "ai.tool_service_unavailable", "AI web tool service is unavailable")
	}

	maxChars := 20000
	if in.MaxCharacters != nil {
		maxChars = *in.MaxCharacters
	}
	args := map[string]any{
		"url":           in.URL,
		"maxCharacters": maxChars,
	}
	res, err := tools.Execute(tc.Context(), aitool.Request{
		OperationID: "fetchWebPage",
		UserID:      tc.User.ID,
		Arguments:   args,
	})
	if err != nil {
		return nil, mapWebToolError(err)
	}
	return normalizeWebToolOutput(res), nil
}

func normalizeWebToolOutput(res aitool.Result) map[string]any {
	out := map[string]any{
		"truncated":    res.Truncated,
		"contentTrust": "untrusted_external",
	}
	if m, ok := res.Value.(map[string]any); ok {
		maps.Copy(out, m)
	} else if res.Value != nil {
		out["data"] = res.Value
	}
	out["truncated"] = res.Truncated
	out["contentTrust"] = "untrusted_external"
	return out
}

func mapWebToolError(err error) error {
	switch {
	case errors.Is(err, aitool.ErrForbidden):
		return aitool.Forbidden("ai.tool_permission_denied", "AI web tool permission was denied")
	case errors.Is(err, aitool.ErrInvalidInput):
		return aitool.BadRequest("request.invalid", "AI web tool arguments are invalid")
	case errors.Is(err, aitool.ErrWebTargetBlocked):
		return aitool.Forbidden("ai.web_target_blocked", "AI web target is blocked by egress policy")
	case errors.Is(err, aitool.ErrWebContentRejected):
		return aitool.NewError(http.StatusUnsupportedMediaType, "ai.web_content_rejected", "AI web content is not readable")
	case errors.Is(err, aitool.ErrWebRequestFailed):
		return aitool.NewError(http.StatusBadGateway, "ai.web_request_failed", "AI web provider request failed")
	default:
		return aitool.NewError(http.StatusInternalServerError, "ai.tool_execution_failed", "AI web tool execution failed")
	}
}

func (h *Handler) executeAdminListAIModels(tc *aitool.ToolContext, _ adminListAIModelsInput) (map[string]any, error) {
	tc.SetNoStore()
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}
	models, err := aimodel.NewService(tc.DB).ListAll(tc.Context())
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "ai.models_unavailable", "AI models are unavailable")
	}
	if models == nil {
		models = []model.AIModel{}
	}
	return map[string]any{
		"items": models,
		"total": len(models),
	}, nil
}

func (h *Handler) executeAdminInspectAgentObservability(tc *aitool.ToolContext, in adminInspectAgentObservabilityInput) (map[string]any, error) {
	tc.SetNoStore()
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}

	view := strings.ToLower(strings.TrimSpace(in.View))
	rangeVal := strings.TrimSpace(derefAIStr(in.Range))
	if rangeVal == "" {
		rangeVal = "1h"
	}
	switch rangeVal {
	case "1h", "6h", "24h", "7d", "30d", "1y":
	default:
		return nil, aitool.BadRequest("ai.observability.range_invalid", "range must be one of 1h, 6h, 24h, 7d, 30d, 1y")
	}

	page, pageSize := aitool.NormalizePagination(in.Page, in.PageSize, 20, 50)
	query := url.Values{}
	query.Set("range", rangeVal)
	query.Set("page", strconv.Itoa(page))
	query.Set("pageSize", strconv.Itoa(pageSize))
	if search := strings.TrimSpace(derefAIStr(in.Search)); search != "" {
		query.Set("search", search)
	}
	if sortBy := strings.TrimSpace(derefAIStr(in.SortBy)); sortBy != "" {
		query.Set("sortBy", sortBy)
	}
	if sortOrder := strings.ToLower(strings.TrimSpace(derefAIStr(in.SortOrder))); sortOrder != "" {
		if sortOrder != "asc" && sortOrder != "desc" {
			return nil, aitool.BadRequest("ai.observability.sort_order_invalid", "sortOrder must be asc or desc")
		}
		query.Set("sortOrder", sortOrder)
	}

	h.host.SetCurrentUser(tc.Gin, tc.User)

	var out map[string]any
	var err error
	switch view {
	case "overview":
		err = tc.InvokeJSON(http.MethodGet, "/api/v1/agent-observability/overview", nil, query, nil, h.GetAgentObservabilityOverview, &out)
	case "conversations":
		err = tc.InvokeJSON(http.MethodGet, "/api/v1/agent-observability/conversations", nil, query, nil, h.ListAgentObservabilityConversations, &out)
	case "turns":
		err = tc.InvokeJSON(http.MethodGet, "/api/v1/agent-observability/turns", nil, query, nil, h.ListAgentObservabilityTurns, &out)
	case "tools":
		err = tc.InvokeJSON(http.MethodGet, "/api/v1/agent-observability/tools", nil, query, nil, h.ListAgentObservabilityTools, &out)
	case "tool_calls":
		opID := strings.TrimSpace(derefAIStr(in.OperationID))
		if opID == "" {
			return nil, aitool.BadRequest("ai.observability.tool_required", "operationId is required when view=tool_calls")
		}
		err = tc.InvokeJSON(
			http.MethodGet,
			"/api/v1/agent-observability/tools/"+url.PathEscape(opID)+"/calls",
			gin.Params{{Key: "operationId", Value: opID}},
			query,
			nil,
			h.ListAgentObservabilityToolCalls,
			&out,
		)
	case "conversation_detail":
		convID := strings.TrimSpace(derefAIStr(in.ConversationID))
		if convID == "" {
			return nil, aitool.BadRequest("ai.observability.conversation_required", "conversationId is required when view=conversation_detail")
		}
		err = tc.InvokeJSON(
			http.MethodGet,
			"/api/v1/agent-observability/conversations/"+url.PathEscape(convID),
			gin.Params{{Key: "conversationId", Value: convID}},
			query,
			nil,
			h.GetAgentObservabilityConversation,
			&out,
		)
	case "trace_detail":
		traceID := strings.TrimSpace(derefAIStr(in.TraceID))
		if traceID == "" {
			return nil, aitool.BadRequest("ai.observability.trace_required", "traceId is required when view=trace_detail")
		}
		err = tc.InvokeJSON(
			http.MethodGet,
			"/api/v1/agent-observability/traces/"+url.PathEscape(traceID),
			gin.Params{{Key: "traceId", Value: traceID}},
			nil,
			nil,
			h.GetAgentObservabilityTrace,
			&out,
		)
	default:
		return nil, aitool.BadRequest("ai.observability.view_invalid", "unsupported observability view")
	}

	if err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	out["view"] = view
	return out, nil
}

func (h *Handler) previewAdminSaveAIModel(tc *aitool.ToolContext, in adminSaveAIModelInput) (*aitool.PreviewResult, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}

	modelRef := strings.TrimSpace(derefAIStr(in.Model))
	if modelRef == "" {
		writeIn, err := buildCreateAIModelWriteInput(in)
		if err != nil {
			return nil, err
		}
		if tc.DB != nil {
			all, lErr := aimodel.NewService(tc.DB).ListAll(tc.Context())
			if lErr != nil {
				return nil, aitool.NewError(http.StatusInternalServerError, "ai.models_unavailable", lErr.Error())
			}
			for _, item := range all {
				if strings.EqualFold(item.Name, writeIn.Name) {
					return nil, aitool.Conflict("ai.model_name_conflict", "AI model name is already in use")
				}
			}
		}
		enabled := writeIn.Enabled == nil || *writeIn.Enabled
		return &aitool.PreviewResult{
			Preview: aitool.ApprovalPreview{
				Summary:        fmt.Sprintf("Create AI model %q (context=%d, maxOutput=%d)", writeIn.Name, writeIn.MaxContextTokens, writeIn.MaxOutputTokens),
				TargetResource: "ai_model",
				Changes: []aitool.PreviewChange{
					{Field: "name", After: writeIn.Name, Action: "create"},
					{Field: "enabled", After: enabled, Action: "create"},
					{Field: "maxContextTokens", After: writeIn.MaxContextTokens, Action: "create"},
					{Field: "maxOutputTokens", After: writeIn.MaxOutputTokens, Action: "create"},
					{Field: "inputCreditsPerMillion", After: writeIn.InputCreditsPerMillion, Action: "create"},
					{Field: "outputCreditsPerMillion", After: writeIn.OutputCreditsPerMillion, Action: "create"},
					{Field: "cachedInputCreditsPerMillion", After: writeIn.CachedInputCreditsPerMillion, Action: "create"},
				},
			},
		}, nil
	}

	if !hasAIModelPatchField(in) {
		return nil, aitool.BadRequest("ai.model_update_noop", "at least one field must be provided when updating an AI model")
	}
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	all, err := aimodel.NewService(tc.DB).ListAll(tc.Context())
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "ai.models_unavailable", err.Error())
	}
	existing, err := aitool.ResolveUniqueByIDOrName(
		all,
		"ai_model",
		"ai.model_not_found",
		modelRef,
		func(m model.AIModel) string { return m.ID },
		func(m model.AIModel) string { return m.Name },
	)
	if err != nil {
		return nil, err
	}

	writeIn, changes, err := mergeAndValidateAIModelPatch(existing, all, in)
	if err != nil {
		return nil, err
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Update AI model %q (%s)", writeIn.Name, existing.ID),
			TargetResource: "ai_model",
			TargetID:       existing.ID,
			Changes:        changes,
		},
		LockedPayload: aiModelTargetLock{ModelID: existing.ID},
	}, nil
}

func (h *Handler) executeAdminSaveAIModel(tc *aitool.ToolContext, in adminSaveAIModelInput) (map[string]any, error) {
	tc.SetNoStore()
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	svc := aimodel.NewService(tc.DB)
	modelRef := strings.TrimSpace(derefAIStr(in.Model))
	if modelRef == "" {
		writeIn, err := buildCreateAIModelWriteInput(in)
		if err != nil {
			return nil, err
		}
		created, err := svc.Create(tc.Context(), writeIn)
		if err != nil {
			h.auditWithContext(tc.User.ID, "ai.model.create", writeIn.Name, false, aimodelAuditCode(err, "ai.model_write_failed"), tc.Context())
			return nil, mapAIModelError(err, "ai.model_write_failed")
		}
		h.auditWithContext(tc.User.ID, "ai.model.create", created.ID, true, "AI model created", tc.Context())
		return aiModelToMap(created), nil
	}

	if !hasAIModelPatchField(in) {
		return nil, aitool.BadRequest("ai.model_update_noop", "at least one field must be provided when updating an AI model")
	}
	all, err := svc.ListAll(tc.Context())
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "ai.models_unavailable", err.Error())
	}
	existing, err := aitool.ResolveUniqueByIDOrName(
		all,
		"ai_model",
		"ai.model_not_found",
		modelRef,
		func(m model.AIModel) string { return m.ID },
		func(m model.AIModel) string { return m.Name },
	)
	if err != nil {
		return nil, err
	}
	if len(tc.LockedPayload) > 0 {
		var locked aiModelTargetLock
		if lErr := tc.UnmarshalLockedPayload(&locked); lErr != nil {
			return nil, aitool.Conflict("ai.preview_lock_invalid", lErr.Error())
		}
		if locked.ModelID != existing.ID {
			return nil, aitool.Conflict("ai.preview_lock_invalid", "target AI model changed since preview")
		}
	}

	writeIn, _, err := mergeAndValidateAIModelPatch(existing, all, in)
	if err != nil {
		return nil, err
	}

	updated, err := svc.Update(tc.Context(), existing.ID, writeIn)
	if err != nil {
		h.auditWithContext(tc.User.ID, "ai.model.update", existing.ID, false, aimodelAuditCode(err, "ai.model_write_failed"), tc.Context())
		return nil, mapAIModelError(err, "ai.model_write_failed")
	}
	h.auditWithContext(tc.User.ID, "ai.model.update", updated.ID, true, "AI model updated", tc.Context())
	return aiModelToMap(updated), nil
}

func (h *Handler) previewAdminDeleteAIModel(tc *aitool.ToolContext, in adminDeleteAIModelInput) (*aitool.PreviewResult, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}
	if strings.TrimSpace(in.Model) == "" {
		return nil, aitool.BadRequest("ai.model_required", "model is required")
	}
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	all, err := aimodel.NewService(tc.DB).ListAll(tc.Context())
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "ai.models_unavailable", err.Error())
	}
	existing, err := aitool.ResolveUniqueByIDOrName(
		all,
		"ai_model",
		"ai.model_not_found",
		in.Model,
		func(m model.AIModel) string { return m.ID },
		func(m model.AIModel) string { return m.Name },
	)
	if err != nil {
		return nil, err
	}
	if existing.Enabled && countOtherEnabledAIModels(all, existing.ID) == 0 {
		return nil, aitool.Conflict("ai.last_model_cannot_be_deleted", "at least one enabled AI model must remain")
	}

	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("Delete AI model %q (%s)", existing.Name, existing.ID),
			TargetResource: "ai_model",
			TargetID:       existing.ID,
			Changes: []aitool.PreviewChange{
				{Field: "model", Before: existing.Name, After: nil, Action: "delete"},
			},
		},
		LockedPayload: aiModelTargetLock{ModelID: existing.ID},
	}, nil
}

func (h *Handler) executeAdminDeleteAIModel(tc *aitool.ToolContext, in adminDeleteAIModelInput) (map[string]any, error) {
	tc.SetNoStore()
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}
	if tc.DB == nil {
		return nil, aitool.NewError(http.StatusServiceUnavailable, "database.unavailable", "database is unavailable")
	}

	svc := aimodel.NewService(tc.DB)
	all, err := svc.ListAll(tc.Context())
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "ai.models_unavailable", err.Error())
	}
	existing, err := aitool.ResolveUniqueByIDOrName(
		all,
		"ai_model",
		"ai.model_not_found",
		in.Model,
		func(m model.AIModel) string { return m.ID },
		func(m model.AIModel) string { return m.Name },
	)
	if err != nil {
		return nil, err
	}
	if len(tc.LockedPayload) > 0 {
		var locked aiModelTargetLock
		if lErr := tc.UnmarshalLockedPayload(&locked); lErr != nil {
			return nil, aitool.Conflict("ai.preview_lock_invalid", lErr.Error())
		}
		if locked.ModelID != existing.ID {
			return nil, aitool.Conflict("ai.preview_lock_invalid", "target AI model changed since preview")
		}
	}

	deleted, err := svc.Delete(tc.Context(), existing.ID)
	if err != nil {
		code := aimodelAuditCode(err, "ai.model_delete_failed")
		h.auditWithContext(tc.User.ID, "ai.model.delete", existing.ID, false, code, tc.Context())
		if errors.Is(err, aimodel.ErrLastEnabled) {
			return nil, aitool.Conflict("ai.last_model_cannot_be_deleted", "at least one enabled AI model must remain")
		}
		return nil, mapAIModelError(err, "ai.model_delete_failed")
	}
	h.auditWithContext(tc.User.ID, "ai.model.delete", deleted.ID, true, "AI model deleted", tc.Context())
	return map[string]any{
		"id":      deleted.ID,
		"name":    deleted.Name,
		"deleted": true,
	}, nil
}

func (h *Handler) previewAdminTestAIIntegration(tc *aitool.ToolContext, in adminTestAIIntegrationInput) (*aitool.PreviewResult, error) {
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}

	target := strings.ToLower(strings.TrimSpace(in.Target))
	switch target {
	case "ai_provider":
		if in.Source != nil || in.URL != nil || in.Token != nil || in.TenantID != nil {
			return nil, aitool.BadRequest("ai.argument_invalid", "source, url, token, and tenantId only apply when target=observability_source")
		}
		if h.aiUnavailableReason() != "" || h.host.AIAgent() == nil {
			return nil, aitool.NewError(http.StatusServiceUnavailable, "ai.agent_unavailable", "AI Agent is unavailable")
		}
		return &aitool.PreviewResult{
			Preview: aitool.ApprovalPreview{
				Summary:        "Test configured AI Provider connectivity through Luna Agent",
				TargetResource: "ai_provider",
			},
		}, nil

	case "observability_source":
		src, effectiveURL, effectiveTenantID, sensitiveKeys, err := h.resolveObservabilityTestParams(in)
		if err != nil {
			return nil, err
		}
		if _, cErr := agentobservability.New(src, agentobservability.Config{
			BaseURL:  effectiveURL,
			TenantID: effectiveTenantID,
		}); cErr != nil {
			return nil, aitool.BadRequest("ai.observability.config_invalid", cErr.Error())
		}
		changes := []aitool.PreviewChange{
			{Field: "source", After: string(src), Action: "test"},
			{Field: "url", After: effectiveURL, Action: "test"},
		}
		if effectiveTenantID != "" {
			changes = append(changes, aitool.PreviewChange{Field: "tenantId", After: effectiveTenantID, Action: "test"})
		}
		return &aitool.PreviewResult{
			Preview: aitool.ApprovalPreview{
				Summary:        fmt.Sprintf("Test %s observability datasource connectivity (%s)", src, effectiveURL),
				TargetResource: "observability_source",
				TargetID:       string(src),
				Changes:        changes,
				SensitiveKeys:  sensitiveKeys,
			},
		}, nil

	default:
		return nil, aitool.BadRequest("ai.argument_invalid", "target must be ai_provider or observability_source")
	}
}

func (h *Handler) executeAdminTestAIIntegration(tc *aitool.ToolContext, in adminTestAIIntegrationInput) (map[string]any, error) {
	tc.SetNoStore()
	if !authz.IsPlatformAdmin(tc.User.Role) {
		return nil, aitool.Forbidden("config.admin.required", "platform administrator role is required")
	}

	target := strings.ToLower(strings.TrimSpace(in.Target))
	switch target {
	case "ai_provider":
		if in.Source != nil || in.URL != nil || in.Token != nil || in.TenantID != nil {
			return nil, aitool.BadRequest("ai.argument_invalid", "source, url, token, and tenantId only apply when target=observability_source")
		}
		if h.aiUnavailableReason() != "" || h.host.AIAgent() == nil {
			return nil, aitool.NewError(http.StatusServiceUnavailable, "ai.agent_unavailable", "AI Agent is unavailable")
		}

		sessionExpiresAt := time.Now().Add(15 * time.Minute).Unix()
		if tc.DB != nil && strings.TrimSpace(tc.SessionID) != "" {
			var session model.UserSession
			if sErr := tc.DB.First(&session, "id = ? AND user_id = ?", tc.SessionID, tc.User.ID).Error; sErr == nil {
				sessionExpiresAt = session.ExpiresAt.Unix()
			}
		}
		actor := aiagent.ActorContext{
			UserID:           tc.User.ID,
			SessionID:        tc.SessionID,
			PlatformRole:     tc.User.Role,
			Locale:           normalizedAILocale(tc.User.Language),
			RequestID:        id.New("req"),
			SessionExpiresAt: sessionExpiresAt,
		}

		resp, err := h.host.AIAgent().Do(tc.Context(), actor, aiagent.Request{
			Method:      http.MethodPost,
			Path:        "/internal/v1/provider/test",
			ContentType: "application/json",
			Body:        []byte(`{}`),
		})
		if err != nil {
			h.auditWithContext(tc.User.ID, "ai.provider.test", "ai.provider", false, "Agent connection test failed", tc.Context())
			return nil, aitool.NewError(http.StatusServiceUnavailable, "ai.provider_unavailable", "AI Provider test failed")
		}
		defer resp.Body.Close()

		var rawBody []byte
		if resp.Body != nil {
			rawBody, _ = io.ReadAll(io.LimitReader(resp.Body, aiErrorBodyLimitBytes))
		}
		okStatus := resp.StatusCode >= 200 && resp.StatusCode < 300
		h.auditWithContext(tc.User.ID, "ai.provider.test", "ai.provider", okStatus, "AI Provider connection tested through Agent", tc.Context())

		if !okStatus {
			code := extractControlledAIErrorCode(rawBody)
			if code == "" {
				code = "ai.provider_unavailable"
			}
			return nil, aitool.NewError(resp.StatusCode, code, "AI Provider test failed")
		}

		out := map[string]any{"target": "ai_provider"}
		var decoded map[string]any
		if len(rawBody) > 0 && json.Unmarshal(rawBody, &decoded) == nil && decoded != nil {
			maps.Copy(out, decoded)
		}
		out["target"] = "ai_provider"
		return out, nil

	case "observability_source":
		src, effectiveURL, effectiveTenantID, _, err := h.resolveObservabilityTestParams(in)
		if err != nil {
			return nil, err
		}
		keys := observabilitySourceKeys[src]
		token := strings.TrimSpace(derefAIStr(in.Token))
		if token == "" && tc.DB != nil {
			token = h.resolveAppConfigSecret(tc.Gin, keys.Token)
		}
		client, cErr := agentobservability.New(src, agentobservability.Config{
			BaseURL:  effectiveURL,
			Token:    token,
			TenantID: effectiveTenantID,
		})
		if cErr != nil {
			return nil, aitool.BadRequest("ai.observability.config_invalid", cErr.Error())
		}
		result, testErr := client.Test(tc.Context())
		h.auditWithContext(tc.User.ID, "ai.observability.test", string(src), testErr == nil, "Agent observability source tested", tc.Context())
		return map[string]any{
			"target":        "observability_source",
			"source":        result.Source,
			"reachable":     result.Reachable,
			"dataAvailable": result.DataAvailable,
			"latencyMs":     result.LatencyMS,
			"code":          result.Code,
		}, nil

	default:
		return nil, aitool.BadRequest("ai.argument_invalid", "target must be ai_provider or observability_source")
	}
}

func (h *Handler) resolveObservabilityTestParams(in adminTestAIIntegrationInput) (agentobservability.Source, string, string, []string, error) {
	rawSource := strings.ToLower(strings.TrimSpace(derefAIStr(in.Source)))
	if rawSource == "" {
		return "", "", "", nil, aitool.BadRequest("ai.observability.source_required", "source is required when target=observability_source")
	}
	src := agentobservability.Source(rawSource)
	keys, valid := observabilitySourceKeys[src]
	if !valid {
		return "", "", "", nil, aitool.BadRequest("ai.observability.source_invalid", "source must be prometheus, loki, or tempo")
	}

	cfgVals := h.allConfigValues()
	effectiveURL := strings.TrimSpace(derefAIStr(in.URL))
	if effectiveURL == "" {
		effectiveURL = strings.TrimSpace(cfgVals[keys.URL])
	}
	if effectiveURL == "" {
		return "", "", "", nil, aitool.NewErrorWithHint(
			http.StatusBadRequest,
			"ai.observability.url_required",
			fmt.Sprintf("%s URL is not configured", src),
			fmt.Sprintf("Provide the url parameter or set %s via admin_update_system_configs first.", keys.URL),
		)
	}

	effectiveTenantID := strings.TrimSpace(derefAIStr(in.TenantID))
	if effectiveTenantID == "" && keys.TenantID != "" {
		effectiveTenantID = strings.TrimSpace(cfgVals[keys.TenantID])
	}

	var sensitiveKeys []string
	if in.Token != nil && strings.TrimSpace(*in.Token) != "" {
		sensitiveKeys = []string{"token"}
	}
	return src, effectiveURL, effectiveTenantID, sensitiveKeys, nil
}

func buildCreateAIModelWriteInput(in adminSaveAIModelInput) (aimodel.WriteInput, error) {
	name := strings.TrimSpace(derefAIStr(in.Name))
	if name == "" {
		return aimodel.WriteInput{}, aitool.BadRequest("ai.model_name_required", "name is required when creating an AI model")
	}
	if in.MaxContextTokens == nil {
		return aimodel.WriteInput{}, aitool.BadRequest("ai.model_context_limit_invalid", "maxContextTokens is required when creating an AI model")
	}
	if *in.MaxContextTokens < aimodel.MinModelContextTokens || *in.MaxContextTokens > aimodel.MaxModelContextTokens {
		return aimodel.WriteInput{}, aitool.BadRequest("ai.model_context_limit_invalid", "maxContextTokens must be between 4096 and 2097152")
	}
	if in.MaxOutputTokens == nil {
		return aimodel.WriteInput{}, aitool.BadRequest("ai.model_output_limit_invalid", "maxOutputTokens is required when creating an AI model")
	}
	if *in.MaxOutputTokens < aimodel.MinModelOutputTokens || *in.MaxOutputTokens > aimodel.MaxModelOutputTokens || *in.MaxOutputTokens >= *in.MaxContextTokens {
		return aimodel.WriteInput{}, aitool.BadRequest("ai.model_output_limit_invalid", "maxOutputTokens must be between 256 and 262144 and strictly less than maxContextTokens")
	}

	for _, price := range []struct {
		val  *string
		code string
		desc string
	}{
		{in.InputCreditsPerMillion, "ai.model_input_price_invalid", "inputCreditsPerMillion"},
		{in.OutputCreditsPerMillion, "ai.model_output_price_invalid", "outputCreditsPerMillion"},
		{in.CachedInputCreditsPerMillion, "ai.model_cached_input_price_invalid", "cachedInputCreditsPerMillion"},
	} {
		raw := strings.TrimSpace(derefAIStr(price.val))
		if raw == "" {
			return aimodel.WriteInput{}, aitool.BadRequest(price.code, price.desc+" is required when creating an AI model")
		}
		if _, err := fixeddecimal.Parse(raw, true, fixeddecimal.Numeric24Scale8Max); err != nil {
			return aimodel.WriteInput{}, aitool.BadRequest(price.code, price.desc+" must be a valid non-negative decimal")
		}
	}

	return aimodel.WriteInput{
		Name:                         name,
		MaxContextTokens:             *in.MaxContextTokens,
		MaxOutputTokens:              *in.MaxOutputTokens,
		InputCreditsPerMillion:       strings.TrimSpace(*in.InputCreditsPerMillion),
		OutputCreditsPerMillion:      strings.TrimSpace(*in.OutputCreditsPerMillion),
		CachedInputCreditsPerMillion: strings.TrimSpace(*in.CachedInputCreditsPerMillion),
		Enabled:                      in.Enabled,
	}, nil
}

func mergeAndValidateAIModelPatch(existing model.AIModel, all []model.AIModel, in adminSaveAIModelInput) (aimodel.WriteInput, []aitool.PreviewChange, error) {
	var changes []aitool.PreviewChange

	nextName := existing.Name
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" {
			return aimodel.WriteInput{}, nil, aitool.BadRequest("ai.model_name_required", "name cannot be empty")
		}
		for _, other := range all {
			if other.ID != existing.ID && strings.EqualFold(other.Name, n) {
				return aimodel.WriteInput{}, nil, aitool.Conflict("ai.model_name_conflict", "AI model name is already in use")
			}
		}
		if n != existing.Name {
			changes = append(changes, aitool.PreviewChange{Field: "name", Before: existing.Name, After: n, Action: "update"})
			nextName = n
		}
	}

	nextCtx := existing.MaxContextTokens
	if in.MaxContextTokens != nil {
		nextCtx = *in.MaxContextTokens
		if nextCtx != existing.MaxContextTokens {
			changes = append(changes, aitool.PreviewChange{Field: "maxContextTokens", Before: existing.MaxContextTokens, After: nextCtx, Action: "update"})
		}
	}
	nextOut := existing.MaxOutputTokens
	if in.MaxOutputTokens != nil {
		nextOut = *in.MaxOutputTokens
		if nextOut != existing.MaxOutputTokens {
			changes = append(changes, aitool.PreviewChange{Field: "maxOutputTokens", Before: existing.MaxOutputTokens, After: nextOut, Action: "update"})
		}
	}
	if nextCtx < aimodel.MinModelContextTokens || nextCtx > aimodel.MaxModelContextTokens {
		return aimodel.WriteInput{}, nil, aitool.BadRequest("ai.model_context_limit_invalid", "maxContextTokens must be between 4096 and 2097152")
	}
	if nextOut < aimodel.MinModelOutputTokens || nextOut > aimodel.MaxModelOutputTokens || nextOut >= nextCtx {
		return aimodel.WriteInput{}, nil, aitool.BadRequest("ai.model_output_limit_invalid", "maxOutputTokens must be between 256 and 262144 and strictly less than maxContextTokens")
	}

	nextInputPrice := existing.InputCreditsPerMillion.String()
	if in.InputCreditsPerMillion != nil {
		parsed, err := fixeddecimal.Parse(*in.InputCreditsPerMillion, true, fixeddecimal.Numeric24Scale8Max)
		if err != nil {
			return aimodel.WriteInput{}, nil, aitool.BadRequest("ai.model_input_price_invalid", "inputCreditsPerMillion must be a non-negative decimal")
		}
		nextInputPrice = parsed.String()
		if !parsed.Equal(existing.InputCreditsPerMillion) {
			changes = append(changes, aitool.PreviewChange{Field: "inputCreditsPerMillion", Before: existing.InputCreditsPerMillion.String(), After: nextInputPrice, Action: "update"})
		}
	}

	nextOutputPrice := existing.OutputCreditsPerMillion.String()
	if in.OutputCreditsPerMillion != nil {
		parsed, err := fixeddecimal.Parse(*in.OutputCreditsPerMillion, true, fixeddecimal.Numeric24Scale8Max)
		if err != nil {
			return aimodel.WriteInput{}, nil, aitool.BadRequest("ai.model_output_price_invalid", "outputCreditsPerMillion must be a non-negative decimal")
		}
		nextOutputPrice = parsed.String()
		if !parsed.Equal(existing.OutputCreditsPerMillion) {
			changes = append(changes, aitool.PreviewChange{Field: "outputCreditsPerMillion", Before: existing.OutputCreditsPerMillion.String(), After: nextOutputPrice, Action: "update"})
		}
	}

	nextCachedPrice := existing.CachedInputCreditsPerMillion.String()
	if in.CachedInputCreditsPerMillion != nil {
		parsed, err := fixeddecimal.Parse(*in.CachedInputCreditsPerMillion, true, fixeddecimal.Numeric24Scale8Max)
		if err != nil {
			return aimodel.WriteInput{}, nil, aitool.BadRequest("ai.model_cached_input_price_invalid", "cachedInputCreditsPerMillion must be a non-negative decimal")
		}
		nextCachedPrice = parsed.String()
		if !parsed.Equal(existing.CachedInputCreditsPerMillion) {
			changes = append(changes, aitool.PreviewChange{Field: "cachedInputCreditsPerMillion", Before: existing.CachedInputCreditsPerMillion.String(), After: nextCachedPrice, Action: "update"})
		}
	}

	nextEnabled := existing.Enabled
	if in.Enabled != nil {
		nextEnabled = *in.Enabled
		if nextEnabled != existing.Enabled {
			changes = append(changes, aitool.PreviewChange{Field: "enabled", Before: existing.Enabled, After: nextEnabled, Action: "update"})
		}
	}
	if existing.Enabled && !nextEnabled && countOtherEnabledAIModels(all, existing.ID) == 0 {
		return aimodel.WriteInput{}, nil, aitool.Conflict("ai.last_model_cannot_be_disabled", "at least one AI model must remain enabled")
	}

	return aimodel.WriteInput{
		Name:                         nextName,
		MaxContextTokens:             nextCtx,
		MaxOutputTokens:              nextOut,
		InputCreditsPerMillion:       nextInputPrice,
		OutputCreditsPerMillion:      nextOutputPrice,
		CachedInputCreditsPerMillion: nextCachedPrice,
		Enabled:                      &nextEnabled,
	}, changes, nil
}

func countOtherEnabledAIModels(all []model.AIModel, excludeID string) int {
	count := 0
	for _, m := range all {
		if m.ID != excludeID && m.Enabled {
			count++
		}
	}
	return count
}

func hasAIModelPatchField(in adminSaveAIModelInput) bool {
	return in.Name != nil ||
		in.Enabled != nil ||
		in.MaxContextTokens != nil ||
		in.MaxOutputTokens != nil ||
		in.InputCreditsPerMillion != nil ||
		in.OutputCreditsPerMillion != nil ||
		in.CachedInputCreditsPerMillion != nil
}

func aimodelAuditCode(err error, fallback string) string {
	if code := aimodel.ErrorCode(err); code != "" {
		return code
	}
	return fallback
}

func mapAIModelError(err error, fallbackCode string) error {
	switch {
	case errors.Is(err, aimodel.ErrNotFound):
		return aitool.NotFound("ai.model_not_found", "AI model was not found")
	case errors.Is(err, aimodel.ErrLastEnabled):
		return aitool.Conflict("ai.last_model_cannot_be_disabled", "at least one AI model must remain enabled")
	case errors.Is(err, aimodel.ErrNameConflict):
		return aitool.Conflict("ai.model_name_conflict", "AI model name is already in use")
	case aimodel.ErrorCode(err) != "":
		return aitool.BadRequest(aimodel.ErrorCode(err), err.Error())
	default:
		return aitool.NewError(http.StatusInternalServerError, fallbackCode, err.Error())
	}
}

func aiModelToMap(m model.AIModel) map[string]any {
	return map[string]any{
		"id":                           m.ID,
		"name":                         m.Name,
		"enabled":                      m.Enabled,
		"maxContextTokens":             m.MaxContextTokens,
		"maxOutputTokens":              m.MaxOutputTokens,
		"inputCreditsPerMillion":       m.InputCreditsPerMillion.String(),
		"outputCreditsPerMillion":      m.OutputCreditsPerMillion.String(),
		"cachedInputCreditsPerMillion": m.CachedInputCreditsPerMillion.String(),
		"createdAt":                    m.CreatedAt,
		"updatedAt":                    m.UpdatedAt,
	}
}

func derefAIStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
