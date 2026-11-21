package aiapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/LiteyukiStudio/devops/docs"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/skills"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	mcpDefaultProtocolVersion = "2025-06-18"
	mcpPreviewLockTTL         = 24 * time.Hour
)

var supportedMCPProtocolVersions = []string{
	"2025-06-18",
	"2025-03-26",
	"2024-11-05",
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type mcpContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpCallToolResult struct {
	Content           []mcpContentBlock `json:"content"`
	StructuredContent any               `json:"structuredContent,omitempty"`
	IsError           bool              `json:"isError,omitempty"`
}

// GetMCPStreamableHTTP handles GET /api/v1/mcp per Streamable HTTP transport §2.2.
func (h *Handler) GetMCPStreamableHTTP(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	if _, _, ok := h.requireMCPBearerAuth(ctx); !ok {
		return
	}
	ctx.Header("Allow", "POST, DELETE")
	ctx.Status(http.StatusMethodNotAllowed)
}

// DeleteMCPStreamableHTTPSession handles DELETE /api/v1/mcp session termination.
func (h *Handler) DeleteMCPStreamableHTTPSession(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	if _, _, ok := h.requireMCPBearerAuth(ctx); !ok {
		return
	}
	ctx.Status(http.StatusNoContent)
}

// PostMCPStreamableHTTP handles POST /api/v1/mcp Streamable HTTP JSON-RPC 2.0 requests and notifications.
func (h *Handler) PostMCPStreamableHTTP(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	user, token, ok := h.requireMCPBearerAuth(ctx)
	if !ok {
		return
	}

	rawBody, err := io.ReadAll(io.LimitReader(ctx.Request.Body, aiRequestBodyLimitBytes+1))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rpcErrorResponse(json.RawMessage("null"), -32700, "Failed to read request body", nil))
		return
	}
	if len(rawBody) > aiRequestBodyLimitBytes {
		ctx.JSON(http.StatusRequestEntityTooLarge, rpcErrorResponse(json.RawMessage("null"), -32600, "Request body exceeds maximum size", nil))
		return
	}
	trimmed := bytes.TrimSpace(rawBody)
	if len(trimmed) == 0 {
		ctx.JSON(http.StatusBadRequest, rpcErrorResponse(json.RawMessage("null"), -32700, "Empty JSON-RPC body", nil))
		return
	}

	if trimmed[0] == '[' {
		var batch []json.RawMessage
		if err := json.Unmarshal(trimmed, &batch); err != nil || len(batch) == 0 {
			ctx.JSON(http.StatusBadRequest, rpcErrorResponse(json.RawMessage("null"), -32700, "Invalid JSON-RPC batch", nil))
			return
		}
		if len(batch) > 32 {
			ctx.JSON(http.StatusBadRequest, rpcErrorResponse(json.RawMessage("null"), -32600, "JSON-RPC batch size exceeds 32", nil))
			return
		}
		responses := make([]jsonRPCResponse, 0, len(batch))
		for _, item := range batch {
			if resp, hasResp := h.dispatchMCPMessage(ctx, user, token, item); hasResp {
				responses = append(responses, resp)
			}
		}
		if len(responses) == 0 {
			ctx.Status(http.StatusAccepted)
			return
		}
		ctx.JSON(http.StatusOK, responses)
		return
	}

	resp, hasResp := h.dispatchMCPMessage(ctx, user, token, trimmed)
	if !hasResp {
		ctx.Status(http.StatusAccepted)
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

func (h *Handler) requireMCPBearerAuth(ctx *gin.Context) (model.User, model.AccessToken, bool) {
	user, token, ok := h.host.AuthenticateMCPBearerToken(ctx)
	if ok {
		return user, token, true
	}
	baseURL := strings.TrimRight(h.host.ExternalBaseURL(ctx), "/")
	if baseURL != "" {
		ctx.Header("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource"`, baseURL))
	} else {
		ctx.Header("WWW-Authenticate", "Bearer")
	}
	telemetry.SetHTTPError(ctx, "auth.token.invalid", "missing or invalid MCP bearer token")
	ctx.JSON(http.StatusUnauthorized, gin.H{
		"code":    "auth.token.invalid",
		"message": "A valid Personal Access Token or OAuth Bearer token is required for /api/v1/mcp",
	})
	return model.User{}, model.AccessToken{}, false
}

func (h *Handler) dispatchMCPMessage(
	ctx *gin.Context,
	user model.User,
	token model.AccessToken,
	raw json.RawMessage,
) (jsonRPCResponse, bool) {
	var req jsonRPCRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return rpcErrorResponse(json.RawMessage("null"), -32700, "Parse error", nil), true
	}
	isNotification := len(bytes.TrimSpace(req.ID)) == 0
	if req.JSONRPC != "2.0" || strings.TrimSpace(req.Method) == "" {
		if isNotification {
			return jsonRPCResponse{}, false
		}
		idVal := req.ID
		if len(idVal) == 0 {
			idVal = json.RawMessage("null")
		}
		return rpcErrorResponse(idVal, -32600, "Invalid JSON-RPC 2.0 request", nil), true
	}
	if isNotification {
		return jsonRPCResponse{}, false
	}

	opName := boundedMCPOperationName(req.Method)
	opCtx, endOp := telemetry.StartOperation(
		ctx.Request.Context(),
		"mcp",
		opName,
		attribute.String("user.id", user.ID),
		attribute.String("luna.mcp.method", opName),
		attribute.String("luna.token.source", token.Source),
	)
	opSpan := trace.SpanFromContext(opCtx)
	restore := replaceRequestContext(ctx, opCtx)
	defer restore()

	var (
		result any
		rpcErr *jsonRPCError
	)
	switch req.Method {
	case "initialize":
		result = h.handleMCPInitialize(req.Params)
	case "ping":
		result = map[string]any{}
	case "tools/list":
		result = map[string]any{"tools": mcpMetaToolDefinitions()}
	case "tools/call":
		result, rpcErr = h.handleMCPToolsCall(ctx, opSpan, user, token, req.Params)
	case "resources/list":
		result, rpcErr = h.handleMCPResourcesList()
	case "resources/read":
		result, rpcErr = h.handleMCPResourcesRead(req.Params)
	case "prompts/list":
		result, rpcErr = h.handleMCPPromptsList()
	case "prompts/get":
		result, rpcErr = h.handleMCPPromptsGet(req.Params)
	default:
		rpcErr = &jsonRPCError{Code: -32601, Message: "Method not found"}
	}

	if rpcErr != nil {
		errCode := fmt.Sprintf("mcp.jsonrpc_%d", rpcErr.Code)
		opSpan.SetAttributes(attribute.String("error.code", errCode))
		endOp(telemetry.WrapError(errCode, "", "mcp."+opName, errors.New(rpcErr.Message)))
		return jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: rpcErr}, true
	}
	if callRes, ok := result.(mcpCallToolResult); ok && callRes.IsError {
		errCode := extractToolResultErrorCode(callRes.StructuredContent)
		if errCode == "" {
			errCode = "ai.tool_execution_failed"
		}
		opSpan.SetAttributes(attribute.String("error.code", errCode))
		endOp(telemetry.WrapError(errCode, "", "mcp."+opName, errors.New(errCode)))
		return jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}, true
	}
	endOp(nil)
	return jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}, true
}

func boundedMCPOperationName(method string) string {
	switch method {
	case "initialize":
		return "initialize"
	case "ping":
		return "ping"
	case "tools/list":
		return "tools_list"
	case "tools/call":
		return "tools_call"
	case "resources/list":
		return "resources_list"
	case "resources/read":
		return "resources_read"
	case "prompts/list":
		return "prompts_list"
	case "prompts/get":
		return "prompts_get"
	default:
		return "unknown"
	}
}

func (h *Handler) handleMCPInitialize(rawParams json.RawMessage) map[string]any {
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(rawParams, &params)
	negotiated := mcpDefaultProtocolVersion
	if slices.Contains(supportedMCPProtocolVersions, strings.TrimSpace(params.ProtocolVersion)) {
		negotiated = strings.TrimSpace(params.ProtocolVersion)
	}
	return map[string]any{
		"protocolVersion": negotiated,
		"capabilities": map[string]any{
			"tools":     map[string]any{"listChanged": false},
			"resources": map[string]any{"subscribe": false, "listChanged": false},
			"prompts":   map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":    "luna-devops",
			"version": h.host.AppVersion(),
		},
		"instructions": "Luna DevOps MCP 采用渐进式领域门面：1) 先按业务域调 load_skill 加载 SOP，再用单域短词调 search_tools 发现工具、写前调 get_tool_details 确认 Schema 与风险分级，涉及内置变量/模板/计费规则先调 search_docs + read_doc；2) L0 直接调 execute_tool，L1 传 confirmed: true，L2 及 stage=prod 强制先调 preview_tool 向用户确认 Diff 后携 previewLock 调 execute_tool；3) 对 pod、previewLock、revision 等易失句柄收到 not_found 或 conflict 时原地重试次数强制为 0，必须退回 project/application/target 稳定锚点（如 runtime_get_target_diagnostics）重对齐；4) 配置写工具均为纯增量补丁，多项 requiresRelease=true 改动须攒齐后合并单次 delivery_deploy_target，观测返回 status=unavailable 时如实报告集群不可达，异步任务按退避节奏有界查询严禁死循环。",
	}
}

func mcpMetaToolDefinitions() []map[string]any {
	return []map[string]any{
		{
			"name":        "load_skill",
			"description": "按需加载 1～3 个 Luna DevOps 业务领域的标准工作流规程（workspace-iam, source-registry-build, application-deployment, runtime-volumes, gateway-topology, diagnostics-governance, platform-admin）。不传 skillIds 时列出全部可用领域。",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"skillIds": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string", "enum": skills.DomainSkillIDs},
						"maxItems":    3,
						"description": "要加载的领域 Skill ID 列表；省略则返回全部领域索引摘要。",
					},
				},
				"additionalProperties": false,
			},
		},
		{
			"name":        "search_tools",
			"description": "按单域简短关键词或过滤条件检索 Luna DevOps 平台工具摘要目录（共 166 个强类型领域工具；非管理员自动过滤 admin 工具），并返回当前令牌 Scope 是否允许调用。",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "单一业务领域的精炼检索词（如 '创建部署目标 环境变量'、'Pod 诊断 日志'）；留空则分页浏览。",
					},
					"domain": map[string]any{
						"type":        "string",
						"enum":        []string{"delivery", "build", "runtime", "network", "workspace", "ops", "admin"},
						"description": "按业务域过滤。",
					},
					"effect": map[string]any{
						"type":        "string",
						"enum":        []string{"read", "write", "async", "execute"},
						"description": "按操作副作用类型过滤。",
					},
					"page":     map[string]any{"type": "integer", "minimum": 1, "default": 1},
					"pageSize": map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "default": 15},
				},
				"additionalProperties": false,
			},
		},
		{
			"name":        "get_tool_details",
			"description": "读取 1～10 个平台工具的完整 JSON 输入/输出 Schema、L0/L1/L2 审批等级、所需令牌 Scope、前置条件与防呆约束。",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"toolNames"},
				"properties": map[string]any{
					"toolNames": map[string]any{
						"type":     "array",
						"items":    map[string]any{"type": "string"},
						"minItems": 1,
						"maxItems": 10,
					},
				},
				"additionalProperties": false,
			},
		},
		{
			"name":        "preview_tool",
			"description": "对平台写工具（尤其是所有 L2 高危/生产/配置/密钥/发布/路由/删除工具）执行服务端 Dry-run 冲突校验，计算脱敏后的 before -> after 变更 Diff，并签发绑定当前用户、令牌与参数哈希的 previewLock。",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"toolName"},
				"properties": map[string]any{
					"toolName":  map[string]any{"type": "string", "description": "目标平台工具名（如 delivery_configure_target_config）。"},
					"arguments": map[string]any{"type": "object", "description": "完整工具参数对象。", "default": map[string]any{}},
				},
				"additionalProperties": false,
			},
		},
		{
			"name":        "execute_tool",
			"description": "执行 Luna DevOps 平台工具。L0 只读工具可直接调用；L1 轻量变更需传 confirmed: true；L2 变更必须先调用 preview_tool 向用户展示 Diff 取得明确同意后，携带返回的 previewLock 调用。",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"toolName"},
				"properties": map[string]any{
					"toolName":       map[string]any{"type": "string", "description": "目标平台工具名。"},
					"arguments":      map[string]any{"type": "object", "description": "完整工具参数对象。", "default": map[string]any{}},
					"previewLock":    map[string]any{"type": "string", "description": "L2 工具必填：由 preview_tool 签发的预检锁令牌。"},
					"confirmed":      map[string]any{"type": "boolean", "description": "L1 轻量写工具确认标记。"},
					"idempotencyKey": map[string]any{"type": "string", "description": "可选幂等键。"},
				},
				"additionalProperties": false,
			},
		},
		{
			"name":        "search_docs",
			"description": "检索随当前平台版本内嵌的公开产品文档（概念、配置语法、内置变量、计费口径与排障指南）。",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"query"},
				"properties": map[string]any{
					"query":  map[string]any{"type": "string", "description": "文档检索关键词。"},
					"locale": map[string]any{"type": "string", "enum": []string{"zh", "en"}, "default": "zh"},
					"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "default": 8},
				},
				"additionalProperties": false,
			},
		},
		{
			"name":        "read_doc",
			"description": "浏览平台文档目录树（省略 path 时）或按路径读取指定文档原文（长文支持用 nextOffset 续读）。",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "文档路径（如 zh/use/mcp/index）；留空返回文档目录列表。"},
					"locale": map[string]any{"type": "string", "enum": []string{"zh", "en"}, "default": "zh"},
					"offset": map[string]any{"type": "integer", "minimum": 0, "default": 0},
				},
				"additionalProperties": false,
			},
		},
	}
}

func (h *Handler) handleMCPToolsCall(
	ctx *gin.Context,
	opSpan trace.Span,
	user model.User,
	token model.AccessToken,
	rawParams json.RawMessage,
) (mcpCallToolResult, *jsonRPCError) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(rawParams, &call); err != nil || strings.TrimSpace(call.Name) == "" {
		return mcpCallToolResult{}, &jsonRPCError{Code: -32602, Message: "Invalid tools/call parameters"}
	}
	metaTool := strings.TrimSpace(call.Name)
	if len(bytes.TrimSpace(call.Arguments)) == 0 {
		call.Arguments = json.RawMessage(`{}`)
	}

	switch metaTool {
	case "load_skill":
		opSpan.SetAttributes(attribute.String("luna.mcp.meta_tool", "load_skill"))
		return h.mcpCallLoadSkill(call.Arguments), nil
	case "search_tools":
		opSpan.SetAttributes(attribute.String("luna.mcp.meta_tool", "search_tools"))
		return h.mcpCallSearchTools(user, token, call.Arguments), nil
	case "get_tool_details":
		opSpan.SetAttributes(attribute.String("luna.mcp.meta_tool", "get_tool_details"))
		return h.mcpCallGetToolDetails(user, token, call.Arguments), nil
	case "preview_tool":
		opSpan.SetAttributes(attribute.String("luna.mcp.meta_tool", "preview_tool"))
		return h.mcpCallPreviewOrExecute(ctx, opSpan, user, token, call.Arguments, true), nil
	case "execute_tool":
		opSpan.SetAttributes(attribute.String("luna.mcp.meta_tool", "execute_tool"))
		return h.mcpCallPreviewOrExecute(ctx, opSpan, user, token, call.Arguments, false), nil
	case "search_docs":
		opSpan.SetAttributes(attribute.String("luna.mcp.meta_tool", "search_docs"))
		return h.mcpCallSearchDocs(ctx, user, call.Arguments), nil
	case "read_doc":
		opSpan.SetAttributes(attribute.String("luna.mcp.meta_tool", "read_doc"))
		return h.mcpCallReadDoc(ctx, user, call.Arguments), nil
	default:
		opSpan.SetAttributes(attribute.String("luna.mcp.meta_tool", "unknown"))
		return mcpToolErrorResult(aitool.BadRequest(
			"mcp.meta_tool_unknown",
			fmt.Sprintf("unknown MCP tool %q; use search_tools, get_tool_details, preview_tool, execute_tool, load_skill, search_docs, or read_doc", metaTool),
		)), nil
	}
}

func (h *Handler) mcpCallLoadSkill(rawArgs json.RawMessage) mcpCallToolResult {
	var in struct {
		SkillIDs []string `json:"skillIds"`
	}
	if err := decodeStrictJSONObject(rawArgs, &in, map[string]bool{"skillIds": true}); err != nil {
		return mcpToolErrorResult(err)
	}
	allDomains, err := skills.ListDomainSkills()
	if err != nil {
		return mcpToolErrorResult(aitool.NewError(http.StatusInternalServerError, "ai.skill_load_failed", "domain skills are unavailable"))
	}
	if len(in.SkillIDs) == 0 {
		summaries := make([]map[string]any, 0, len(allDomains))
		for _, item := range allDomains {
			summaries = append(summaries, map[string]any{
				"skillId":     item.ID,
				"description": item.Description,
			})
		}
		return mcpToolSuccessResult(map[string]any{"availableSkills": summaries})
	}
	if len(in.SkillIDs) > 3 {
		return mcpToolErrorResult(aitool.BadRequest("ai.tool_arguments_invalid", "at most 3 skillIds may be loaded at once"))
	}
	seen := map[string]bool{}
	items := make([]map[string]any, 0, len(in.SkillIDs))
	for _, rawID := range in.SkillIDs {
		skillID := strings.TrimSpace(rawID)
		if seen[skillID] {
			continue
		}
		seen[skillID] = true
		entry, ok, getErr := skills.GetDomainSkill(skillID)
		if getErr != nil || !ok {
			return mcpToolErrorResult(aitool.BadRequest(
				"ai.skill_not_found",
				fmt.Sprintf("unknown skillId %q; allowed values: %s", skillID, strings.Join(skills.DomainSkillIDs, ", ")),
			))
		}
		items = append(items, map[string]any{
			"skillId":      entry.ID,
			"description":  entry.Description,
			"instructions": entry.Instructions,
		})
	}
	return mcpToolSuccessResult(map[string]any{"items": items})
}

func (h *Handler) mcpCallSearchTools(user model.User, token model.AccessToken, rawArgs json.RawMessage) mcpCallToolResult {
	var in struct {
		Query    string `json:"query"`
		Domain   string `json:"domain"`
		Effect   string `json:"effect"`
		Page     int    `json:"page"`
		PageSize int    `json:"pageSize"`
	}
	allowed := map[string]bool{"query": true, "domain": true, "effect": true, "page": true, "pageSize": true}
	if err := decodeStrictJSONObject(rawArgs, &in, allowed); err != nil {
		return mcpToolErrorResult(err)
	}
	if in.Page <= 0 {
		in.Page = 1
	}
	if in.PageSize <= 0 {
		in.PageSize = 15
	}
	if in.PageSize > 50 {
		in.PageSize = 50
	}

	registry := h.host.AIToolRegistry()
	if registry == nil {
		return mcpToolErrorResult(aitool.NewError(http.StatusServiceUnavailable, "ai.tool_catalog_unavailable", "AI tool registry is unavailable"))
	}

	isAdmin := authz.IsPlatformAdmin(user.Role)
	domainFilter := strings.ToLower(strings.TrimSpace(in.Domain))
	effectFilter := strings.ToLower(strings.TrimSpace(in.Effect))
	queryTokens := tokenizeMCPQuery(in.Query)
	rawQueryLower := strings.ToLower(strings.TrimSpace(in.Query))

	type scoredOp struct {
		op    aitool.CatalogOperation
		score int
	}
	candidates := make([]scoredOp, 0, 166)
	for _, cat := range registry.Catalog() {
		if cat.AdminOnly && !isAdmin {
			continue
		}
		if domainFilter != "" && strings.ToLower(cat.Domain) != domainFilter {
			continue
		}
		if effectFilter != "" && strings.ToLower(cat.Effect) != effectFilter {
			continue
		}
		score := scoreCatalogOperation(cat, rawQueryLower, queryTokens)
		if len(queryTokens) > 0 && score == 0 {
			continue
		}
		candidates = append(candidates, scoredOp{op: cat, score: score})
	}

	if len(queryTokens) > 0 {
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].score != candidates[j].score {
				return candidates[i].score > candidates[j].score
			}
			return candidates[i].op.OperationID < candidates[j].op.OperationID
		})
	}

	total := len(candidates)
	start := (in.Page - 1) * in.PageSize
	if start > total {
		start = total
	}
	end := start + in.PageSize
	if end > total {
		end = total
	}

	items := make([]map[string]any, 0, end-start)
	for _, item := range candidates[start:end] {
		missing := missingTokenScopes(token.Scope, item.op.RequiredScopes)
		items = append(items, map[string]any{
			"toolName":          item.op.OperationID,
			"domain":            item.op.Domain,
			"summary":           item.op.Summary,
			"effect":            item.op.Effect,
			"approvalTier":      item.op.ApprovalTier,
			"requiredScopes":    item.op.RequiredScopes,
			"tokenScopeAllowed": len(missing) == 0,
		})
	}
	totalPages := 0
	if total > 0 {
		totalPages = (total + in.PageSize - 1) / in.PageSize
	}
	return mcpToolSuccessResult(map[string]any{
		"items":      items,
		"page":       in.Page,
		"pageSize":   in.PageSize,
		"total":      total,
		"totalPages": totalPages,
	})
}

func (h *Handler) mcpCallGetToolDetails(user model.User, token model.AccessToken, rawArgs json.RawMessage) mcpCallToolResult {
	var in struct {
		ToolNames []string `json:"toolNames"`
	}
	if err := decodeStrictJSONObject(rawArgs, &in, map[string]bool{"toolNames": true}); err != nil {
		return mcpToolErrorResult(err)
	}
	if len(in.ToolNames) == 0 || len(in.ToolNames) > 10 {
		return mcpToolErrorResult(aitool.BadRequest("ai.tool_arguments_invalid", "toolNames must contain between 1 and 10 tool names"))
	}
	registry := h.host.AIToolRegistry()
	if registry == nil {
		return mcpToolErrorResult(aitool.NewError(http.StatusServiceUnavailable, "ai.tool_catalog_unavailable", "AI tool registry is unavailable"))
	}
	isAdmin := authz.IsPlatformAdmin(user.Role)
	items := make([]map[string]any, 0, len(in.ToolNames))
	notFound := make([]string, 0)
	for _, rawName := range in.ToolNames {
		name := strings.TrimSpace(rawName)
		tool, ok := registry.Get(name)
		if !ok || (tool.Catalog.AdminOnly && !isAdmin) {
			notFound = append(notFound, name)
			continue
		}
		cat := tool.Catalog
		missing := missingTokenScopes(token.Scope, cat.RequiredScopes)
		entry := map[string]any{
			"toolName":          cat.OperationID,
			"domain":            cat.Domain,
			"summary":           cat.Summary,
			"effect":            cat.Effect,
			"approvalTier":      cat.ApprovalTier,
			"requiresPreview":   cat.ApprovalTier == aitool.ApprovalTierL2,
			"idempotent":        cat.Idempotent,
			"requiredScopes":    cat.RequiredScopes,
			"tokenScopeAllowed": len(missing) == 0,
			"purpose":           cat.Purpose,
			"avoidWhen":         cat.AvoidWhen,
			"preconditions":     cat.Preconditions,
			"successEvidence":   cat.SuccessEvidence,
			"inputSchema":       cat.InputSchema,
			"outputSchema":      cat.OutputSchema,
		}
		if len(missing) > 0 {
			entry["missingScopes"] = missing
		}
		items = append(items, entry)
	}
	return mcpToolSuccessResult(map[string]any{
		"items":    items,
		"notFound": notFound,
	})
}

func (h *Handler) mcpCallPreviewOrExecute(
	ctx *gin.Context,
	opSpan trace.Span,
	user model.User,
	token model.AccessToken,
	rawArgs json.RawMessage,
	isPreview bool,
) mcpCallToolResult {
	var in struct {
		ToolName       string          `json:"toolName"`
		Arguments      json.RawMessage `json:"arguments"`
		PreviewLock    string          `json:"previewLock"`
		Confirmed      bool            `json:"confirmed"`
		IdempotencyKey string          `json:"idempotencyKey"`
	}
	allowed := map[string]bool{"toolName": true, "arguments": true}
	if !isPreview {
		allowed["previewLock"] = true
		allowed["confirmed"] = true
		allowed["idempotencyKey"] = true
	}
	if err := decodeStrictJSONObject(rawArgs, &in, allowed); err != nil {
		return mcpToolErrorResult(err)
	}
	toolName := strings.TrimSpace(in.ToolName)
	if toolName == "" {
		return mcpToolErrorResult(aitool.BadRequest("ai.tool_not_available", "toolName is required"))
	}
	if len(bytes.TrimSpace(in.Arguments)) == 0 {
		in.Arguments = json.RawMessage(`{}`)
	}

	registry := h.host.AIToolRegistry()
	if registry == nil {
		return mcpToolErrorResult(aitool.NewError(http.StatusServiceUnavailable, "ai.tool_catalog_unavailable", "AI tool registry is unavailable"))
	}
	tool, ok := registry.Get(toolName)
	if !ok {
		return mcpToolErrorResult(aitool.NotFound("ai.tool_not_available", fmt.Sprintf("tool %q is not registered", toolName)))
	}
	opSpan.SetAttributes(attribute.String("luna.tool.name", tool.Catalog.OperationID))

	if tool.Catalog.AdminOnly && !authz.IsPlatformAdmin(user.Role) {
		return mcpToolErrorResult(aitool.Forbidden("auth.forbidden", "platform administrator role is required for this tool"))
	}

	missingScopes := missingTokenScopes(token.Scope, tool.Catalog.RequiredScopes)
	if len(missingScopes) > 0 {
		err := aitool.Forbidden(
			"auth.token.scope_insufficient",
			fmt.Sprintf("access token is missing required scope %q for %s", missingScopes[0], tool.Catalog.OperationID),
		)
		err.Details = map[string]any{
			"requiredScopes": tool.Catalog.RequiredScopes,
			"missingScopes":  missingScopes,
		}
		return mcpToolErrorResult(err)
	}

	effectiveArgs, argsMap, sandboxErr := h.enforceConversationProjectSandbox(ctx, tool, in.Arguments, "")
	if sandboxErr != nil {
		return mcpToolErrorResult(sandboxErr)
	}
	wireArgsHash, hashErr := aitool.CanonicalJSONHash(in.Arguments)
	if hashErr != nil {
		return mcpToolErrorResult(aitool.BadRequest("ai.tool_arguments_invalid", "tool arguments are not valid JSON"))
	}

	isProdTarget := h.isTargetProductionStage(ctx, "", argsMap)
	effectiveTier := resolveEffectiveApprovalTier(tool.Catalog, argsMap, isProdTarget)
	mcpRunBindingID := "mcp:" + token.ID
	signingSecret := h.host.MCPPreviewSigningSecret()

	var lockedPayload json.RawMessage
	if !isPreview {
		switch effectiveTier {
		case aitool.ApprovalTierL0:
			// Direct execution permitted.
		case aitool.ApprovalTierL1:
			if strings.TrimSpace(in.PreviewLock) != "" {
				payload, verifyErr := aitool.VerifyPreviewLock(
					[][]byte{signingSecret},
					in.PreviewLock,
					aitool.PreviewLockClaims{
						RunID:         mcpRunBindingID,
						ToolCallID:    "mcp",
						OperationID:   tool.Catalog.OperationID,
						ArgumentsHash: wireArgsHash,
						OwnerUserID:   user.ID,
					},
					time.Now(),
				)
				if verifyErr != nil {
					return mcpToolErrorResult(mapPreviewLockVerifyError(verifyErr))
				}
				lockedPayload = payload
			} else if !in.Confirmed {
				err := aitool.Forbidden(
					"ai.tool_confirmation_required",
					fmt.Sprintf("tool %q is tier L1; confirm parameters with the user and pass confirmed: true (or previewLock)", tool.Catalog.OperationID),
				)
				err.Hint = "Confirm the target and parameters with the user, then call execute_tool with confirmed: true."
				return mcpToolErrorResult(err)
			}
		default:
			if strings.TrimSpace(in.PreviewLock) == "" {
				err := aitool.Forbidden(
					"ai.tool_preview_lock_required",
					fmt.Sprintf("tool %q is tier L2 and requires a two-step preview lock before execution", tool.Catalog.OperationID),
				)
				err.Hint = "Call preview_tool first, present the returned approvalPreview diff to the user for explicit confirmation, then call execute_tool with the exact same arguments and previewLock."
				return mcpToolErrorResult(err)
			}
			payload, verifyErr := aitool.VerifyPreviewLock(
				[][]byte{signingSecret},
				in.PreviewLock,
				aitool.PreviewLockClaims{
					RunID:         mcpRunBindingID,
					ToolCallID:    "mcp",
					OperationID:   tool.Catalog.OperationID,
					ArgumentsHash: wireArgsHash,
					OwnerUserID:   user.ID,
				},
				time.Now(),
			)
			if verifyErr != nil {
				return mcpToolErrorResult(mapPreviewLockVerifyError(verifyErr))
			}
			lockedPayload = payload
		}
	}

	mcpCallID := id.New("mcpc")
	tc := aitool.NewToolContext(
		ctx,
		h.dbFor(ctx),
		user,
		"mcp:"+token.ID,
		"mcp",
		"",
		mcpRunBindingID,
		mcpCallID,
		strings.TrimSpace(in.IdempotencyKey),
		"mcp",
		lockedPayload,
	)

	if isPreview {
		var previewRes aitool.PreviewResult
		if tool.HasPreview {
			res, previewErr := tool.Preview(tc, effectiveArgs)
			if previewErr != nil {
				return mcpToolErrorResult(previewErr)
			}
			if res != nil {
				previewRes = *res
			}
		} else {
			previewRes = aitool.PreviewResult{
				Preview: aitool.ApprovalPreview{
					Summary:        fmt.Sprintf("Preview %s (%s)", tool.Catalog.OperationID, effectiveTier),
					TargetResource: tool.Catalog.Domain,
				},
			}
		}
		now := time.Now()
		claims := aitool.PreviewLockClaims{
			RunID:         mcpRunBindingID,
			ToolCallID:    "mcp",
			OperationID:   tool.Catalog.OperationID,
			ArgumentsHash: wireArgsHash,
			OwnerUserID:   user.ID,
			IssuedAt:      now.Unix(),
			ExpiresAt:     now.Add(mcpPreviewLockTTL).Unix(),
		}
		lockToken, signErr := aitool.SignPreviewLock(signingSecret, claims, previewRes.LockedPayload)
		if signErr != nil {
			return mcpToolErrorResult(aitool.NewError(http.StatusInternalServerError, "ai.preview_lock_sign_failed", "failed to sign preview lock"))
		}
		return mcpToolSuccessResult(map[string]any{
			"toolName":        tool.Catalog.OperationID,
			"approvalTier":    effectiveTier,
			"approvalPreview": previewRes.Preview,
			"previewLock":     lockToken,
		})
	}

	out, execErr := tool.Execute(tc, effectiveArgs)
	if effectiveTier != aitool.ApprovalTierL0 || tool.Catalog.Effect != aitool.EffectRead || slices.Contains(tool.Catalog.RequiredScopes, "web:read") {
		h.auditWithContext(
			user.ID,
			"mcp.tool.execute",
			tool.Catalog.OperationID+":"+mcpCallID,
			execErr == nil,
			fmt.Sprintf("MCP %s bearer operation executed", token.Source),
			ctx.Request.Context(),
		)
	}
	if execErr != nil {
		return mcpToolErrorResult(execErr)
	}
	if out == nil {
		out = map[string]any{"ok": true}
	}
	return mcpToolSuccessResult(out)
}

func (h *Handler) mcpCallSearchDocs(ctx *gin.Context, user model.User, rawArgs json.RawMessage) mcpCallToolResult {
	var in struct {
		Query  string `json:"query"`
		Locale string `json:"locale"`
		Limit  int    `json:"limit"`
	}
	if err := decodeStrictJSONObject(rawArgs, &in, map[string]bool{"query": true, "locale": true, "limit": true}); err != nil {
		return mcpToolErrorResult(err)
	}
	if strings.TrimSpace(in.Query) == "" {
		return mcpToolErrorResult(aitool.BadRequest("ai.tool_arguments_invalid", "query is required"))
	}
	locale := strings.TrimSpace(in.Locale)
	if locale == "" {
		locale = user.Language
	}
	hits := docs.Search(in.Query, locale, "", in.Limit)
	_ = ctx
	return mcpToolSuccessResult(map[string]any{
		"locale": docs.NormalizeLocale(locale),
		"items":  hits,
	})
}

func (h *Handler) mcpCallReadDoc(ctx *gin.Context, user model.User, rawArgs json.RawMessage) mcpCallToolResult {
	var in struct {
		Path   string `json:"path"`
		Locale string `json:"locale"`
		Offset int    `json:"offset"`
	}
	if err := decodeStrictJSONObject(rawArgs, &in, map[string]bool{"path": true, "locale": true, "offset": true}); err != nil {
		return mcpToolErrorResult(err)
	}
	locale := strings.TrimSpace(in.Locale)
	if locale == "" {
		locale = user.Language
	}
	res, ok := docs.Read(in.Path, locale, "", in.Offset)
	_ = ctx
	if !ok {
		return mcpToolErrorResult(aitool.NotFound("docs.not_found", fmt.Sprintf("documentation path %q was not found", in.Path)))
	}
	return mcpToolSuccessResult(res)
}

func (h *Handler) handleMCPResourcesList() (any, *jsonRPCError) {
	entry, err := skills.EntrySkill()
	if err != nil {
		return nil, &jsonRPCError{Code: -32603, Message: "Skills catalog unavailable"}
	}
	domains, err := skills.ListDomainSkills()
	if err != nil {
		return nil, &jsonRPCError{Code: -32603, Message: "Skills catalog unavailable"}
	}
	resources := make([]map[string]any, 0, 1+len(domains))
	resources = append(resources, map[string]any{
		"uri":         "luna://skills/luna-devops",
		"name":        entry.Name,
		"description": entry.Description,
		"mimeType":    "text/markdown",
	})
	for _, item := range domains {
		resources = append(resources, map[string]any{
			"uri":         "luna://skills/domains/" + item.ID,
			"name":        item.Name,
			"description": item.Description,
			"mimeType":    "text/markdown",
		})
	}
	return map[string]any{"resources": resources}, nil
}

func (h *Handler) handleMCPResourcesRead(rawParams json.RawMessage) (any, *jsonRPCError) {
	var in struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(rawParams, &in); err != nil || strings.TrimSpace(in.URI) == "" {
		return nil, &jsonRPCError{Code: -32602, Message: "uri is required"}
	}
	uri := strings.TrimSpace(in.URI)
	if uri == "luna://skills/luna-devops" {
		entry, err := skills.EntrySkill()
		if err != nil {
			return nil, &jsonRPCError{Code: -32603, Message: "Skill unavailable"}
		}
		return map[string]any{
			"contents": []map[string]any{{
				"uri":      uri,
				"mimeType": "text/markdown",
				"text":     entry.RawMarkdown,
			}},
		}, nil
	}
	if strings.HasPrefix(uri, "luna://skills/domains/") {
		skillID := strings.TrimPrefix(uri, "luna://skills/domains/")
		entry, ok, err := skills.GetDomainSkill(skillID)
		if err != nil || !ok {
			return nil, &jsonRPCError{Code: -32602, Message: "Unknown skill resource URI"}
		}
		return map[string]any{
			"contents": []map[string]any{{
				"uri":      uri,
				"mimeType": "text/markdown",
				"text":     entry.RawMarkdown,
			}},
		}, nil
	}
	return nil, &jsonRPCError{Code: -32602, Message: "Unsupported resource URI"}
}

func (h *Handler) handleMCPPromptsList() (any, *jsonRPCError) {
	entry, err := skills.EntrySkill()
	if err != nil {
		return nil, &jsonRPCError{Code: -32603, Message: "Prompts unavailable"}
	}
	domains, err := skills.ListDomainSkills()
	if err != nil {
		return nil, &jsonRPCError{Code: -32603, Message: "Prompts unavailable"}
	}
	prompts := make([]map[string]any, 0, 1+len(domains))
	prompts = append(prompts, map[string]any{
		"name":        entry.ID,
		"description": entry.Description,
	})
	for _, item := range domains {
		prompts = append(prompts, map[string]any{
			"name":        item.ID,
			"description": item.Description,
		})
	}
	return map[string]any{"prompts": prompts}, nil
}

func (h *Handler) handleMCPPromptsGet(rawParams json.RawMessage) (any, *jsonRPCError) {
	var in struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rawParams, &in); err != nil || strings.TrimSpace(in.Name) == "" {
		return nil, &jsonRPCError{Code: -32602, Message: "name is required"}
	}
	name := strings.TrimSpace(in.Name)
	var entry skills.SkillEntry
	if name == "luna-devops" {
		loaded, err := skills.EntrySkill()
		if err != nil {
			return nil, &jsonRPCError{Code: -32603, Message: "Prompt unavailable"}
		}
		entry = loaded
	} else {
		loaded, ok, err := skills.GetDomainSkill(name)
		if err != nil || !ok {
			return nil, &jsonRPCError{Code: -32602, Message: "Unknown prompt name"}
		}
		entry = loaded
	}
	return map[string]any{
		"description": entry.Description,
		"messages": []map[string]any{{
			"role": "user",
			"content": map[string]any{
				"type": "text",
				"text": entry.Instructions,
			},
		}},
	}, nil
}

func missingTokenScopes(tokenScopeText string, requiredScopes []string) []string {
	var missing []string
	for _, req := range requiredScopes {
		req = strings.TrimSpace(req)
		if req == "" {
			continue
		}
		if !authz.AccessTokenAllows(tokenScopeText, req) {
			missing = append(missing, req)
		}
	}
	return missing
}

func decodeStrictJSONObject(raw json.RawMessage, dst any, allowedKeys map[string]bool) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		trimmed = []byte(`{}`)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil || obj == nil {
		return aitool.BadRequest("ai.tool_arguments_invalid", "arguments must be a JSON object")
	}
	for k := range obj {
		if !allowedKeys[k] {
			return aitool.BadRequest("ai.tool_arguments_invalid", "unknown argument "+k)
		}
	}
	if err := json.Unmarshal(trimmed, dst); err != nil {
		return aitool.BadRequest("ai.tool_arguments_invalid", "invalid argument types")
	}
	return nil
}

func mapPreviewLockVerifyError(err error) *aitool.ToolError {
	switch {
	case errors.Is(err, aitool.ErrPreviewLockExpired):
		return aitool.NewErrorWithHint(
			http.StatusConflict,
			"ai.preview_lock_expired",
			"previewLock has expired",
			"Call preview_tool again to inspect the latest state and obtain a fresh previewLock.",
		)
	case errors.Is(err, aitool.ErrPreviewLockMismatch):
		return aitool.NewErrorWithHint(
			http.StatusConflict,
			"ai.preview_lock_mismatch",
			"previewLock does not match the current user, token, toolName, or arguments",
			"Do not modify arguments after preview_tool; call preview_tool again if arguments changed.",
		)
	default:
		return aitool.Forbidden("ai.preview_lock_invalid", "previewLock signature is invalid")
	}
}

func mcpToolSuccessResult(payload any) mcpCallToolResult {
	var rawBytes []byte
	switch v := payload.(type) {
	case json.RawMessage:
		rawBytes = v
		var decoded any
		if json.Unmarshal(v, &decoded) == nil {
			payload = decoded
		}
	default:
		rawBytes, _ = json.Marshal(payload)
	}
	return mcpCallToolResult{
		Content:           []mcpContentBlock{{Type: "text", Text: string(rawBytes)}},
		StructuredContent: payload,
		IsError:           false,
	}
}

func mcpToolErrorResult(err error) mcpCallToolResult {
	errObj := map[string]any{
		"code":    "ai.tool_execution_failed",
		"message": "tool execution failed",
	}
	if te, ok := aitool.AsToolError(err); ok {
		errObj["code"] = te.Code
		errObj["message"] = te.Message
		if te.Hint != "" {
			errObj["hint"] = te.Hint
		}
		if len(te.Candidates) > 0 {
			errObj["candidates"] = te.Candidates
		}
		if len(te.Details) > 0 {
			errObj["details"] = te.Details
		}
	} else if err != nil {
		errObj["message"] = err.Error()
	}
	rawBytes, _ := json.Marshal(errObj)
	return mcpCallToolResult{
		Content:           []mcpContentBlock{{Type: "text", Text: string(rawBytes)}},
		StructuredContent: errObj,
		IsError:           true,
	}
}

func extractToolResultErrorCode(structured any) string {
	if m, ok := structured.(map[string]any); ok {
		if code, ok := m["code"].(string); ok {
			return code
		}
	}
	return ""
}

func rpcErrorResponse(idVal json.RawMessage, code int, message string, data any) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      idVal,
		Error:   &jsonRPCError{Code: code, Message: message, Data: data},
	}
}

func tokenizeMCPQuery(query string) []string {
	lower := strings.ToLower(strings.TrimSpace(query))
	if lower == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(tok string) {
		tok = strings.TrimSpace(tok)
		if tok == "" || seen[tok] {
			return
		}
		seen[tok] = true
		out = append(out, tok)
	}
	var asciiBuf []rune
	var cjkBuf []rune
	flushASCII := func() {
		if len(asciiBuf) >= 2 {
			word := string(asciiBuf)
			add(word)
			for _, part := range strings.FieldsFunc(word, func(r rune) bool { return r == '_' || r == '-' }) {
				if len(part) >= 2 {
					add(part)
				}
			}
		}
		asciiBuf = asciiBuf[:0]
	}
	flushCJK := func() {
		if len(cjkBuf) == 1 {
			add(string(cjkBuf))
		} else if len(cjkBuf) >= 2 {
			for i := 0; i < len(cjkBuf)-1; i++ {
				add(string(cjkBuf[i : i+2]))
			}
		}
		cjkBuf = cjkBuf[:0]
	}
	for _, r := range lower {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-':
			flushCJK()
			asciiBuf = append(asciiBuf, r)
		case unicode.Is(unicode.Han, r):
			flushASCII()
			cjkBuf = append(cjkBuf, r)
		default:
			flushASCII()
			flushCJK()
		}
	}
	flushASCII()
	flushCJK()
	return out
}

func scoreCatalogOperation(cat aitool.CatalogOperation, rawLower string, tokens []string) int {
	if len(tokens) == 0 {
		return 1
	}
	idLower := strings.ToLower(cat.OperationID)
	summaryLower := strings.ToLower(cat.Summary)
	domainLower := strings.ToLower(cat.Domain)
	tagsLower := strings.ToLower(strings.Join(cat.Tags, " "))
	aliasesLower := strings.ToLower(strings.Join(cat.Aliases.ZH, " ") + " " + strings.Join(cat.Aliases.EN, " "))
	purposeLower := strings.ToLower(cat.Purpose.ZH + " " + cat.Purpose.EN)

	score := 0
	if rawLower != "" && idLower == rawLower {
		score += 80
	} else if rawLower != "" && strings.Contains(idLower, rawLower) {
		score += 30
	}
	for _, tok := range tokens {
		if idLower == tok {
			score += 40
		} else if strings.Contains(idLower, tok) {
			score += 10
		}
		if domainLower == tok {
			score += 8
		}
		if strings.Contains(summaryLower, tok) {
			score += 9
		}
		if strings.Contains(aliasesLower, tok) {
			score += 7
		}
		if strings.Contains(tagsLower, tok) {
			score += 6
		}
		if strings.Contains(purposeLower, tok) {
			score += 3
		}
	}
	return score
}
