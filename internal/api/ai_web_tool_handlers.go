package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) ExecuteAIWebSearch(ctx *gin.Context) {
	h.executeAIWebTool(ctx, "webSearch")
}

func (h *Handlers) ExecuteAIFetchWebPage(ctx *gin.Context) {
	h.executeAIWebTool(ctx, "fetchWebPage")
}

func (h *Handlers) executeAIWebTool(ctx *gin.Context, operationID string) {
	actor, bound := ctx.Request.Context().Value(aiPlatformActorContextKey{}).(aiPlatformActor)
	if !bound {
		writeErrorCode(ctx, http.StatusForbidden, "ai.tool_service_identity_required", "AI web tools require an active service-bound ToolCall")
		return
	}
	user, resolved := h.currentAIPlatformUser(ctx)
	if !resolved || user.ID == "" {
		return
	}

	var arguments map[string]any
	if !bindJSON(ctx, &arguments) {
		return
	}
	if h.aiTools == nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.tool_service_unavailable", "AI web tool service is unavailable")
		return
	}

	result, err := h.aiTools.Execute(ctx.Request.Context(), aitool.Request{
		OperationID: operationID,
		UserID:      user.ID,
		SessionID:   actor.SessionID,
		Arguments:   arguments,
	})
	if err != nil {
		h.writeAIWebToolError(ctx, operationID, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"data": result.Value, "truncated": result.Truncated})
}

func (h *Handlers) writeAIWebToolError(ctx *gin.Context, operationID string, err error) {
	switch {
	case errors.Is(err, aitool.ErrForbidden):
		writeErrorCode(ctx, http.StatusForbidden, "ai.tool_permission_denied", "AI web tool permission was denied")
	case errors.Is(err, aitool.ErrInvalidInput):
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid", "AI web tool arguments are invalid")
	case errors.Is(err, aitool.ErrWebTargetBlocked):
		writeErrorCode(ctx, http.StatusForbidden, "ai.web_target_blocked", "AI web target is blocked")
	case errors.Is(err, aitool.ErrWebContentRejected):
		writeErrorCode(ctx, http.StatusUnsupportedMediaType, "ai.web_content_rejected", "AI web content is not readable")
	case errors.Is(err, aitool.ErrWebRequestFailed):
		telemetry.LogWarn(ctx.Request.Context(), "AI web tool request failed",
			"ai.tool.web_request.failed", operationID, "provider.request.failed", err,
			slog.String("request_id", requestID(ctx)))
		telemetry.MarkHTTPErrorLogged(ctx)
		writeErrorCode(ctx, http.StatusBadGateway, "ai.web_request_failed", "AI web provider request failed")
	default:
		telemetry.LogError(ctx.Request.Context(), "AI web tool execution failed",
			"ai.tool.execution.failed", operationID, "agent.tool.failed", err,
			slog.String("request_id", requestID(ctx)))
		telemetry.MarkHTTPErrorLogged(ctx)
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.tool_execution_failed", "AI web tool execution failed")
	}
}
