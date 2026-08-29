package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/agentobservability"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
)

const (
	agentObservabilityEnabledKey = "ai.observability.enabled"
)

var observabilitySourceKeys = map[agentobservability.Source]struct {
	URL      string
	Token    string
	TenantID string
}{
	agentobservability.SourcePrometheus: {URL: "ai.observability.prometheus_url", Token: "ai.observability.prometheus_token"},
	agentobservability.SourceLoki:       {URL: "ai.observability.loki_url", Token: "ai.observability.loki_token", TenantID: "ai.observability.loki_tenant_id"},
	agentobservability.SourceTempo:      {URL: "ai.observability.tempo_url", Token: "ai.observability.tempo_token", TenantID: "ai.observability.tempo_tenant_id"},
}

type observabilityTestInput struct {
	Source   agentobservability.Source `json:"source" binding:"required"`
	URL      string                    `json:"url" binding:"required"`
	Token    string                    `json:"token"`
	TenantID string                    `json:"tenantId"`
}

type agentObservabilityOverview struct {
	GeneratedAt     time.Time                            `json:"generatedAt"`
	Range           string                               `json:"range"`
	Summary         agentObservabilitySummary            `json:"summary"`
	SourceStatus    map[agentobservability.Source]string `json:"sourceStatus"`
	ObservationCode string                               `json:"observationCode"`
}

type agentObservabilitySummary struct {
	InputTokens           int64    `json:"inputTokens"`
	OutputTokens          int64    `json:"outputTokens"`
	CacheReadInputTokens  *int64   `json:"cacheReadInputTokens"`
	CacheWriteInputTokens *int64   `json:"cacheWriteInputTokens"`
	ReasoningOutputTokens *int64   `json:"reasoningOutputTokens"`
	CacheHitRate          *float64 `json:"cacheHitRate"`
	ToolCalls             float64  `json:"toolCalls"`
	ToolSuccessRate       float64  `json:"toolSuccessRate"`
	TurnCount             int64    `json:"turnCount"`
	TurnSuccessRate       float64  `json:"turnSuccessRate"`
	RunDurationP95        float64  `json:"runDurationP95"`
}

func (h *Handlers) TestAgentObservabilitySource(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input observabilityTestInput
	if !bindJSON(ctx, &input) {
		return
	}
	keys, valid := observabilitySourceKeys[input.Source]
	if !valid {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.observability.source_invalid", "unsupported observability source")
		return
	}
	token := strings.TrimSpace(input.Token)
	if token == "" {
		token = h.resolveAppConfigSecret(ctx, keys.Token)
	}
	client, err := agentobservability.New(input.Source, agentobservability.Config{
		BaseURL: input.URL, Token: token, TenantID: input.TenantID,
	})
	if err != nil {
		ctx.JSON(http.StatusOK, agentobservability.TestResult{Source: input.Source, Code: "ai.observability.config_invalid"})
		return
	}
	result, testErr := client.Test(ctx.Request.Context())
	h.auditWithContext(user.ID, "ai.observability.test", string(input.Source), testErr == nil, "Agent observability source tested", ctx.Request.Context())
	// A failed test is a diagnostic result, not a failed configuration mutation.
	ctx.JSON(http.StatusOK, result)
}

func (h *Handlers) GetAgentObservabilityOverview(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	values := h.configs.get(knownConfigKeys())
	if !configBool(values[agentObservabilityEnabledKey]) {
		writeAgentObservabilityUnavailable(ctx, "ai.observability.disabled", "Agent observability is disabled")
		return
	}
	rangeText, duration := observabilityRange(ctx.Query("range"))
	end := time.Now()
	start := end.Add(-duration)
	keys := observabilitySourceKeys[agentobservability.SourcePrometheus]
	client, err := agentobservability.New(agentobservability.SourcePrometheus, agentobservability.Config{
		BaseURL: values[keys.URL], Token: h.resolveAppConfigSecret(ctx, keys.Token), TenantID: values[keys.TenantID],
	})
	if err != nil {
		writeAgentObservabilityUnavailable(ctx, "ai.observability.not_configured", "Prometheus is not configured")
		return
	}

	result := agentObservabilityOverview{
		GeneratedAt: end, Range: rangeText,
		SourceStatus:    map[agentobservability.Source]string{agentobservability.SourcePrometheus: "ready"},
		ObservationCode: "ai.observability.ready",
	}
	queries := agentObservabilitySummaryQueries(rangeText)
	series, queryErr := client.Query(ctx.Request.Context(), queries["runDurationP95"], end)
	if queryErr != nil {
		result.SourceStatus[agentobservability.SourcePrometheus] = "unavailable"
		result.ObservationCode = "ai.observability.partial"
	} else {
		result.Summary.RunDurationP95 = firstSeriesValue(series)
	}
	store := agentobservability.NewConversationStore(h.dbFor(ctx))
	turnSummary, err := store.SummarizeTurns(ctx.Request.Context(), start)
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.observability.turns_failed", "Agent turns are unavailable")
		return
	}
	toolSummary, err := store.SummarizeTools(ctx.Request.Context(), start)
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.observability.tools_failed", "Agent tool summaries are unavailable")
		return
	}
	runSummary, err := store.SummarizeRuns(ctx.Request.Context(), start)
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.observability.runs_failed", "Agent run summaries are unavailable")
		return
	}
	result.Summary.InputTokens = runSummary.InputTokens
	result.Summary.OutputTokens = runSummary.OutputTokens
	result.Summary.CacheReadInputTokens = runSummary.CacheReadInputTokens
	result.Summary.CacheWriteInputTokens = runSummary.CacheWriteInputTokens
	result.Summary.ReasoningOutputTokens = runSummary.ReasoningOutputTokens
	result.Summary.CacheHitRate = runSummary.CacheHitRate
	result.Summary.RunDurationP95 = runSummary.DurationP95Seconds
	result.Summary.TurnCount = turnSummary.Total
	result.Summary.TurnSuccessRate = turnSummary.SuccessRate
	result.Summary.ToolCalls = float64(toolSummary.Total)
	result.Summary.ToolSuccessRate = toolSummary.SuccessRate
	ctx.JSON(http.StatusOK, result)
}

func agentObservabilitySummaryQueries(rangeText string) map[string]string {
	return map[string]string{
		"runDurationP95": fmt.Sprintf(`histogram_quantile(0.95, sum(increase(luna_devops_agent_run_duration_seconds_bucket[%s])) by (le)) or vector(0)`, rangeText),
	}
}

func (h *Handlers) GetAgentObservabilityTrace(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	values := h.configs.get(knownConfigKeys())
	if !configBool(values[agentObservabilityEnabledKey]) {
		writeAgentObservabilityUnavailable(ctx, "ai.observability.disabled", "Agent observability is disabled")
		return
	}
	keys := observabilitySourceKeys[agentobservability.SourceTempo]
	client, err := agentobservability.New(agentobservability.SourceTempo, agentobservability.Config{
		BaseURL: values[keys.URL], Token: h.resolveAppConfigSecret(ctx, keys.Token), TenantID: values[keys.TenantID],
	})
	if err != nil {
		writeAgentObservabilityUnavailable(ctx, "ai.observability.not_configured", "Tempo is not configured")
		return
	}
	detail, err := client.GetTrace(ctx.Request.Context(), ctx.Param("traceId"))
	if err != nil {
		writeAgentObservabilityUnavailable(ctx, "ai.observability.trace_unavailable", "Trace detail is unavailable")
		return
	}
	traceContext, contextErr := agentobservability.NewConversationStore(h.dbFor(ctx)).FindTraceContext(ctx.Request.Context(), detail.TraceID)
	if contextErr == nil {
		detail.Context = traceContext
	}
	h.auditWithContext(user.ID, "ai.observability.trace.view", detail.TraceID, true, "Agent observability trace viewed", ctx.Request.Context())
	ctx.JSON(http.StatusOK, detail)
}

func (h *Handlers) ListAgentObservabilityConversations(ctx *gin.Context) {
	user, ok := h.requireAgentObservabilityAdmin(ctx)
	if !ok {
		return
	}
	rangeText, duration := observabilityRange(ctx.Query("range"))
	pagination := paginationFromQuery(ctx)
	result, err := agentobservability.NewConversationStore(h.dbFor(ctx)).List(ctx.Request.Context(), agentobservability.ConversationListOptions{
		Start: time.Now().Add(-duration), Search: ctx.Query("search"), Page: pagination.Page, PageSize: pagination.PageSize,
		SortBy: pagination.SortBy, SortOrder: pagination.SortOrder,
	})
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.observability.conversations_failed", "Agent conversations are unavailable")
		return
	}
	h.auditWithContext(user.ID, "ai.observability.conversations.list", rangeText, true, "Agent observability conversations listed", ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{
		"items": result.Items, "page": result.Page, "pageSize": result.PageSize, "sortBy": result.SortBy,
		"sortOrder": result.SortOrder, "total": result.Total, "totalPages": result.TotalPages,
	})
}

func (h *Handlers) ListAgentObservabilityTurns(ctx *gin.Context) {
	user, ok := h.requireAgentObservabilityAdmin(ctx)
	if !ok {
		return
	}
	rangeText, duration := observabilityRange(ctx.Query("range"))
	pagination := paginationFromQuery(ctx)
	result, err := agentobservability.NewConversationStore(h.dbFor(ctx)).ListTurns(ctx.Request.Context(), agentobservability.ConversationListOptions{
		Start: time.Now().Add(-duration), Search: ctx.Query("search"), Page: pagination.Page, PageSize: pagination.PageSize,
		SortBy: pagination.SortBy, SortOrder: pagination.SortOrder,
	})
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.observability.turns_failed", "Agent turns are unavailable")
		return
	}
	h.auditWithContext(user.ID, "ai.observability.turns.list", rangeText, true, "Agent observability turns listed", ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{
		"items": result.Items, "page": result.Page, "pageSize": result.PageSize, "sortBy": result.SortBy,
		"sortOrder": result.SortOrder, "total": result.Total, "totalPages": result.TotalPages,
	})
}

func (h *Handlers) ListAgentObservabilityTools(ctx *gin.Context) {
	_, ok := h.requireAgentObservabilityAdmin(ctx)
	if !ok {
		return
	}
	_, duration := observabilityRange(ctx.Query("range"))
	pagination := paginationFromQuery(ctx)
	result, err := agentobservability.NewConversationStore(h.dbFor(ctx)).ListToolSummaries(ctx.Request.Context(), agentobservability.ToolSummaryListOptions{
		Start: time.Now().Add(-duration), Search: ctx.Query("search"), Page: pagination.Page, PageSize: pagination.PageSize,
		SortBy: pagination.SortBy, SortOrder: pagination.SortOrder,
	})
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.observability.tools_failed", "Agent tool summaries are unavailable")
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"items": result.Items, "page": result.Page, "pageSize": result.PageSize, "sortBy": result.SortBy,
		"sortOrder": result.SortOrder, "total": result.Total, "totalPages": result.TotalPages,
	})
}

func (h *Handlers) ListAgentObservabilityToolCalls(ctx *gin.Context) {
	user, ok := h.requireAgentObservabilityAdmin(ctx)
	if !ok {
		return
	}
	operationID := strings.TrimSpace(ctx.Param("operationId"))
	if operationID == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.observability.tool_required", "Agent tool operation is required")
		return
	}
	rangeText, duration := observabilityRange(ctx.Query("range"))
	pagination := paginationFromQuery(ctx)
	result, err := agentobservability.NewConversationStore(h.dbFor(ctx)).ListToolCalls(ctx.Request.Context(), agentobservability.ToolCallListOptions{
		Start: time.Now().Add(-duration), OperationID: operationID, Page: pagination.Page, PageSize: pagination.PageSize,
		SortBy: pagination.SortBy, SortOrder: pagination.SortOrder,
	})
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.observability.tool_calls_failed", "Agent tool calls are unavailable")
		return
	}
	h.auditWithContext(user.ID, "ai.observability.tool_calls.list", operationID, true, "Agent observability tool calls listed for "+rangeText, ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{
		"items": result.Items, "page": result.Page, "pageSize": result.PageSize, "sortBy": result.SortBy,
		"sortOrder": result.SortOrder, "total": result.Total, "totalPages": result.TotalPages,
	})
}

func (h *Handlers) GetAgentObservabilityConversation(ctx *gin.Context) {
	user, ok := h.requireAgentObservabilityAdmin(ctx)
	if !ok {
		return
	}
	conversationID := strings.TrimSpace(ctx.Param("conversationId"))
	pagination := paginationFromQuery(ctx)
	detail, err := agentobservability.NewConversationStore(h.dbFor(ctx)).Get(ctx.Request.Context(), conversationID, pagination.Page, pagination.PageSize)
	if errors.Is(err, agentobservability.ErrConversationNotFound) {
		writeErrorCode(ctx, http.StatusNotFound, "ai.observability.conversation_not_found", "Agent conversation was not found")
		return
	}
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.observability.conversation_failed", "Agent conversation is unavailable")
		return
	}
	h.auditWithContext(user.ID, "ai.observability.conversation.view", conversationID, true, "Agent observability conversation viewed", ctx.Request.Context())
	ctx.JSON(http.StatusOK, detail)
}

func (h *Handlers) requireAgentObservabilityAdmin(ctx *gin.Context) (model.User, bool) {
	ctx.Header("Cache-Control", "no-store")
	user, ok := h.currentUser(ctx)
	if !ok {
		return model.User{}, false
	}
	values := h.configs.get(knownConfigKeys())
	if !configBool(values[agentObservabilityEnabledKey]) {
		writeAgentObservabilityUnavailable(ctx, "ai.observability.disabled", "Agent observability is disabled")
		return model.User{}, false
	}
	return user, true
}

func writeAgentObservabilityUnavailable(ctx *gin.Context, code, detail string) {
	ctx.Header("Cache-Control", "no-store")
	telemetry.SetHTTPError(ctx, code, detail)
	response := errorEnvelope(ctx, http.StatusServiceUnavailable, code)
	response["retryable"] = true
	response["status"] = "unavailable"
	response["observationCode"] = code
	if isDevelopmentRequest(ctx) {
		response["developerDetail"] = telemetry.RedactText(detail)
	}
	ctx.JSON(http.StatusServiceUnavailable, response)
}

func (h *Handlers) resolveAppConfigSecret(ctx *gin.Context, key string) string {
	if key == "" {
		return ""
	}
	var row model.AppConfig
	if err := h.dbFor(ctx).First(&row, "key = ?", key).Error; err != nil {
		return ""
	}
	return h.secrets.ResolveContext(ctx.Request.Context(), row.Value)
}

func observabilityRange(value string) (string, time.Duration) {
	switch value {
	case "6h":
		return "6h", 6 * time.Hour
	case "24h":
		return "24h", 24 * time.Hour
	case "7d":
		return "7d", 7 * 24 * time.Hour
	case "30d":
		return "30d", 30 * 24 * time.Hour
	case "1y":
		return "1y", 365 * 24 * time.Hour
	default:
		return "1h", time.Hour
	}
}

func firstSeriesValue(series []agentobservability.Series) float64 {
	if len(series) == 0 || len(series[0].Points) == 0 {
		return 0
	}
	return series[0].Points[len(series[0].Points)-1].Value
}
