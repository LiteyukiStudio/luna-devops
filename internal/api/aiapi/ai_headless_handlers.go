package aiapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type aiHeadlessToolRequest struct {
	ActorUserID string          `json:"actorUserId"`
	ProjectID   string          `json:"projectId"`
	ToolName    string          `json:"toolName"`
	Arguments   json.RawMessage `json:"arguments"`
}

// ExecuteHeadlessAIToolInternal handles POST /internal/v1/ai/tools/execute-headless.
// It executes strictly L0 read-only tools on behalf of a headless AI inference task
// without requiring interactive ai.runs/ai.conversations/user_sessions rows, while
// enforcing live user status, AI access policy, RBAC scopes, and project sandboxing.
func (h *Handler) ExecuteHeadlessAIToolInternal(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	if !h.requireAIAgentService(ctx) {
		return
	}

	runID := strings.TrimSpace(ctx.GetHeader(aiRunIDHeader))
	toolCallID := strings.TrimSpace(ctx.GetHeader(aiToolCallIDHeader))
	if runID == "" || toolCallID == "" || len(runID) > 128 || len(toolCallID) > 128 {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.tool_execution_binding_invalid", "headless AI tool execution binding is incomplete")
		return
	}

	var req aiHeadlessToolRequest
	if !bindJSON(ctx, &req) {
		return
	}
	actorUserID := strings.TrimSpace(req.ActorUserID)
	toolName := strings.TrimSpace(req.ToolName)
	projectID := strings.TrimSpace(req.ProjectID)
	if actorUserID == "" || toolName == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.tool_arguments_invalid", "actorUserId and toolName are required")
		return
	}
	if len(bytes.TrimSpace(req.Arguments)) == 0 {
		req.Arguments = json.RawMessage(`{}`)
	}

	span := trace.SpanFromContext(ctx.Request.Context())
	span.SetAttributes(
		attribute.String("luna.run.id", runID),
		attribute.String("luna.tool_call.id", toolCallID),
		attribute.String("luna.tool.name", toolName),
	)

	reg := h.host.AIToolRegistry()
	if reg == nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.tool_catalog_unavailable", "AI tool registry is unavailable")
		return
	}
	tool, ok := reg.Get(toolName)
	if !ok {
		writeErrorCode(ctx, http.StatusForbidden, "ai.tool_not_available", "requested tool is not registered")
		return
	}
	if tool.Catalog.ApprovalTier != aitool.ApprovalTierL0 ||
		tool.Catalog.Effect != aitool.EffectRead ||
		tool.Catalog.VerificationOnly {
		writeErrorCode(ctx, http.StatusForbidden, "ai.headless_tier_exceeded", "headless tool gateway only permits L0 read-only tools")
		return
	}

	var user model.User
	if err := h.dbFor(ctx).First(&user, "id = ? and disabled = ?", actorUserID, false).Error; err != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
		return
	}
	if !h.aiAccessAllowed(user.Role) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.user_not_allowed", "AI assistant is restricted to platform administrators")
		return
	}
	if tool.Catalog.AdminOnly && !authz.IsPlatformAdmin(user.Role) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.tool_permission_denied", "platform administrator role is required for this tool")
		return
	}
	if !aiOperationScopesAllowed(user.Role, tool.Catalog.RequiredScopes) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.tool_permission_denied", "AI tool operation exceeds the user's authorized scopes")
		return
	}

	effectiveArgs, _, sandboxErr := h.enforceConversationProjectSandbox(ctx, tool, req.Arguments, projectID)
	if sandboxErr != nil {
		writeFacadeToolError(ctx, sandboxErr)
		return
	}

	h.host.SetCurrentUser(ctx, user)
	tc := aitool.NewToolContext(
		ctx,
		h.dbFor(ctx),
		user,
		"headless:"+user.ID,
		"headless",
		projectID,
		runID,
		toolCallID,
		"headless:"+toolCallID,
		"headless",
		nil,
	)

	result, execErr := tool.Execute(tc, effectiveArgs)
	if execErr != nil {
		span.SetStatus(codes.Error, "headless_tool_failed")
		writeFacadeToolError(ctx, execErr)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (h *Handler) runHeadlessInferenceForUser(
	ctx *gin.Context,
	user model.User,
	input aiagent.HeadlessInferenceRequest,
) (*aiagent.HeadlessInferenceResponse, bool) {
	if reason := h.aiUnavailableReason(); reason != "" {
		status := http.StatusServiceUnavailable
		if reason == "ai.disabled" {
			status = http.StatusConflict
		}
		writeAIProxyError(ctx, status, reason, "AI assistant is unavailable")
		return nil, false
	}
	if !h.aiAccessAllowed(user.Role) {
		writeAIProxyError(ctx, http.StatusForbidden, "ai.user_not_allowed", "AI assistant is restricted to platform administrators")
		return nil, false
	}
	var liveUser model.User
	if user.Disabled || h.dbFor(ctx).First(&liveUser, "id = ? and disabled = ?", user.ID, false).Error != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
		return nil, false
	}
	if !h.aiAccessAllowed(liveUser.Role) {
		writeAIProxyError(ctx, http.StatusForbidden, "ai.user_not_allowed", "AI assistant is restricted to platform administrators")
		return nil, false
	}
	if err := (billing.Service{DB: h.dbFor(ctx)}).EnsureAIAvailableBalance(ctx.Request.Context(), liveUser.ID); err != nil {
		if errors.Is(err, billing.ErrAIWalletBalanceInsufficient) {
			writeAIProxyError(ctx, http.StatusUnprocessableEntity, "ai.wallet_balance_insufficient", "Personal wallet balance is insufficient for AI inference")
			return nil, false
		}
		writeErrorCode(ctx, http.StatusServiceUnavailable, "billing.wallet_unavailable", "failed to verify personal wallet balance")
		return nil, false
	}

	locale := strings.TrimSpace(requestLanguage(ctx))
	if locale == "" {
		locale = normalizedAILocale(liveUser.Language)
	} else {
		locale = normalizedAILocale(locale)
	}

	actor := aiagent.ActorContext{
		UserID:       liveUser.ID,
		SessionID:    "headless:" + liveUser.ID,
		PlatformRole: liveUser.Role,
		ProjectID:    strings.TrimSpace(input.ProjectID),
		Locale:       locale,
		RequestID:    id.New("req"),
	}
	resp, err := aiagent.RunHeadlessInference(ctx.Request.Context(), h.host.AIAgent(), actor, input)
	if err != nil {
		var upErr *aiagent.UpstreamError
		if errors.As(err, &upErr) {
			code := normalizeHeadlessUpstreamCode(upErr.Code)
			status := upErr.StatusCode
			if status < http.StatusBadRequest || status > 599 {
				status = http.StatusBadGateway
			}
			writeAIProxyError(ctx, status, code, "AI inference request failed")
			return nil, false
		}
		writeAIProxyError(ctx, http.StatusServiceUnavailable, "ai.agent_unavailable", err.Error())
		return nil, false
	}
	return resp, true
}

func normalizeHeadlessUpstreamCode(raw string) string {
	candidate := strings.TrimSpace(raw)
	if candidate == "invalid_request" {
		return "request.invalid"
	}
	if candidate == "" || candidate == "ai.internal_error" || !aiControlledUpstreamError.MatchString(candidate) {
		return "ai.upstream_failed"
	}
	return candidate
}
