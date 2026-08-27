package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

const (
	aiAssistantEnabledConfigKey = "ai.assistant.enabled"
	aiAccessModeConfigKey       = "ai.access.mode"
	aiTextInputLimitBytes       = 128 * 1024
	aiRequestBodyLimitBytes     = 1024 * 1024
)

var aiToolActionOperationID = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.-]{2,100}$`)

type aiProxyRoute struct {
	method   string
	internal string
	stream   bool
	status   int
	validate func(*Handlers, *gin.Context, model.User, []byte) bool
}

type aiCapabilitiesResponse struct {
	Enabled       bool `json:"enabled"`
	MaxInputBytes int  `json:"maxInputBytes"`
}

func (h *Handlers) GetAICapabilities(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	_, role, ok := h.aiActorFromSession(ctx)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, aiCapabilitiesResponse{
		Enabled:       h.aiAssistantEnabled() && h.aiAccessAllowed(role),
		MaxInputBytes: aiTextInputLimitBytes,
	})
}

func (h *Handlers) ProxyAIRequest(ctx *gin.Context) {
	route, ok := aiRouteFor(ctx)
	if !ok {
		writeErrorCode(ctx, http.StatusNotFound, "resource.not_found", "AI route not found")
		return
	}
	actor, role, ok := h.aiActorFromSession(ctx)
	if !ok {
		return
	}
	if !h.aiAccessAllowed(role) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.user_not_allowed", "AI assistant access is restricted to platform administrators")
		return
	}
	if reason := h.aiUnavailableReason(); reason != "" {
		writeErrorCode(ctx, http.StatusServiceUnavailable, reason, "AI assistant is unavailable")
		return
	}

	body, ok := h.readAIBody(ctx)
	if !ok {
		return
	}
	if route.validate != nil && !route.validate(h, ctx, model.User{ID: actor.UserID, Language: actor.Locale}, body) {
		return
	}
	if ctx.FullPath() == "/api/v1/ai/conversations/:conversationId/turns" {
		var attached bool
		body, attached = h.attachAIModelSnapshot(ctx, actor.UserID, body)
		if !attached {
			return
		}
	}

	actor.ProjectID = projectIDFromAIRequest(ctx, body)
	actor.RunID = strings.TrimSpace(ctx.Param("runId"))
	if ctx.FullPath() == "/api/v1/ai/conversations/:conversationId/turns" || ctx.FullPath() == "/api/v1/ai/conversations/:conversationId/tool-actions" {
		var prepared bool
		body, actor, prepared = prepareAITurn(ctx, actor, body)
		if !prepared {
			return
		}
	}
	contentType := ctx.GetHeader("Content-Type")
	if len(body) == 0 {
		contentType = ""
	}
	response, err := h.aiAgent.Do(ctx.Request.Context(), actor, aiagent.Request{
		Method:         route.method,
		Path:           expandAIInternalPath(route.internal, ctx),
		Query:          cloneAIQuery(ctx.Request.URL.Query()),
		Body:           body,
		ContentType:    contentType,
		LastEventID:    strings.TrimSpace(ctx.GetHeader("Last-Event-ID")),
		IdempotencyKey: strings.TrimSpace(ctx.GetHeader("Idempotency-Key")),
		Stream:         route.stream,
	})
	if err != nil {
		if errors.Is(err, aiagent.ErrUnavailable) {
			writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.agent_unavailable", "AI agent is unavailable")
			return
		}
		writeErrorCode(ctx, http.StatusBadGateway, "ai.agent_unavailable", "AI agent request failed")
		return
	}
	defer response.Body.Close()
	h.copyAIResponse(ctx, response, route.status, "ai.agent_unavailable")
}

func (h *Handlers) attachAIModelSnapshot(ctx *gin.Context, userID string, body []byte) ([]byte, bool) {
	var input map[string]any
	if json.Unmarshal(body, &input) != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid_json", "invalid turn input")
		return nil, false
	}
	modelID, ok := input["modelId"].(string)
	if !ok || strings.TrimSpace(modelID) == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.model_required", "AI model is required")
		return nil, false
	}
	db := h.dbFor(ctx)
	if db == nil {
		return body, true
	}
	if _, err := (billing.Service{DB: db}).EnsureWallet(userID); err != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "billing.wallet_unavailable", "personal wallet is unavailable")
		return nil, false
	}
	var selected model.AIModel
	if err := db.Where("id = ? AND enabled = ?", strings.TrimSpace(modelID), true).First(&selected).Error; err != nil {
		writeErrorCode(ctx, http.StatusConflict, "ai.model_not_available", "selected AI model is unavailable")
		return nil, false
	}
	input["modelSnapshot"] = gin.H{
		"id":                           selected.ID,
		"name":                         selected.Name,
		"maxContextTokens":             selected.MaxContextTokens,
		"maxOutputTokens":              selected.MaxOutputTokens,
		"inputCreditsPerMillion":       selected.InputCreditsPerMillion,
		"outputCreditsPerMillion":      selected.OutputCreditsPerMillion,
		"cachedInputCreditsPerMillion": selected.CachedInputCreditsPerMillion,
	}
	prepared, err := json.Marshal(input)
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.run_create_failed", "cannot prepare AI model selection")
		return nil, false
	}
	return prepared, true
}

func (h *Handlers) aiActorFromSession(ctx *gin.Context) (aiagent.ActorContext, string, bool) {
	if h.aiActorResolver != nil {
		return h.aiActorResolver(ctx)
	}
	session, ok := h.currentSessionFromCookie(ctx)
	if !ok {
		writeErrorCode(ctx, http.StatusUnauthorized, "auth.session.missing", "a browser session is required")
		return aiagent.ActorContext{}, "", false
	}
	var user model.User
	if h.dbFor(ctx) == nil || h.dbFor(ctx).First(&user, "id = ? and disabled = ?", session.UserID, false).Error != nil {
		writeErrorCode(ctx, http.StatusUnauthorized, "auth.session.expired", "the browser session is invalid")
		return aiagent.ActorContext{}, "", false
	}
	return aiagent.ActorContext{
		UserID: user.ID, SessionID: session.ID, Locale: normalizedAILocale(user.Language),
		RequestID: id.New("req"), SessionExpiresAt: session.ExpiresAt.Unix(),
	}, user.Role, true
}

func (h *Handlers) aiAccessAllowed(role string) bool {
	if h.configs == nil {
		return false
	}
	mode := strings.TrimSpace(h.configs.get([]string{aiAccessModeConfigKey})[aiAccessModeConfigKey])
	switch mode {
	case "all_authenticated":
		return authz.IsPlatformRole(role)
	case "admins":
		return authz.IsPlatformAdmin(role)
	default:
		return false
	}
}

func prepareAITurn(ctx *gin.Context, actor aiagent.ActorContext, body []byte) ([]byte, aiagent.ActorContext, bool) {
	var input map[string]any
	if json.Unmarshal(body, &input) != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid_json", "invalid turn input")
		return nil, actor, false
	}
	now := time.Now()
	input["pageContext"] = enrichAIPageContext(input["pageContext"], actor, now)
	runID := id.New("airun")
	input["runId"] = runID
	prepared, err := json.Marshal(input)
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.run_create_failed", "cannot prepare AI run")
		return nil, actor, false
	}
	actor.RunID = runID
	return prepared, actor, true
}

func enrichAIPageContext(raw any, actor aiagent.ActorContext, now time.Time) map[string]any {
	pageContext, _ := raw.(map[string]any)
	if pageContext == nil {
		pageContext = map[string]any{}
	}
	pageContext["locale"] = normalizedAILocale(actor.Locale)
	pageContext["server"] = map[string]any{
		"requestTimestamp":      now.UTC().Format(time.RFC3339),
		"locale":                normalizedAILocale(actor.Locale),
		"projectContextPresent": actor.ProjectID != "",
	}
	return pageContext
}

func (h *Handlers) aiUnavailableReason() string {
	if !h.aiAssistantEnabled() {
		return "ai.disabled"
	}
	if h.aiAgent == nil {
		return "ai.agent_unavailable"
	}
	return ""
}

func (h *Handlers) aiAssistantEnabled() bool {
	if !h.aiDeploymentEnabled || h.configs == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(h.configs.get([]string{aiAssistantEnabledConfigKey})[aiAssistantEnabledConfigKey]), "true")
}

func (h *Handlers) copyAIResponse(ctx *gin.Context, response *aiagent.Response, fallbackStatus int, errorCode string) {
	status := response.StatusCode
	if status == 0 {
		status = fallbackStatus
	}
	if (status < http.StatusOK || status >= http.StatusMultipleChoices) && config.RuntimeMode() == "production" {
		ctx.Header("Cache-Control", "no-store")
		if retryAfter := response.Header.Get("Retry-After"); retryAfter != "" {
			ctx.Header("Retry-After", retryAfter)
		}
		writeErrorCode(ctx, status, errorCode, "AI upstream request failed")
		return
	}
	for _, header := range []string{"Content-Type", "Cache-Control", "ETag", "Retry-After"} {
		if value := response.Header.Get(header); value != "" {
			ctx.Header(header, value)
		}
	}
	if strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		ctx.Header("Cache-Control", "no-cache, no-transform")
		ctx.Header("X-Accel-Buffering", "no")
		ctx.Status(status)
		buffer := make([]byte, 4096)
		for {
			count, readErr := response.Body.Read(buffer)
			if count > 0 {
				if _, writeErr := ctx.Writer.Write(buffer[:count]); writeErr != nil {
					return
				}
				ctx.Writer.Flush()
			}
			if readErr != nil {
				return
			}
		}
	}
	ctx.Status(status)
	_, _ = io.Copy(ctx.Writer, response.Body)
}

func (h *Handlers) readAIBody(ctx *gin.Context) ([]byte, bool) {
	if ctx.Request.Body == nil {
		return nil, true
	}
	body, err := io.ReadAll(io.LimitReader(ctx.Request.Body, aiRequestBodyLimitBytes+1))
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid", "cannot read request body")
		return nil, false
	}
	if len(body) > aiRequestBodyLimitBytes {
		writeErrorCode(ctx, http.StatusRequestEntityTooLarge, "ai.input_too_large", "AI request body exceeds the limit")
		return nil, false
	}
	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	return body, true
}

func validateCreateAIConversation(h *Handlers, ctx *gin.Context, user model.User, body []byte) bool {
	var input struct {
		ProjectID string `json:"projectId"`
		ModelID   string `json:"modelId"`
		Title     string `json:"title"`
	}
	if len(body) == 0 || json.Unmarshal(body, &input) != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid_json", "invalid conversation input")
		return false
	}
	if len([]byte(input.Title)) > 200 {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.title_too_large", "conversation title is too large")
		return false
	}
	if !validateEnabledAIModel(h, ctx, input.ModelID) {
		return false
	}
	if input.ProjectID != "" {
		if _, ok := h.findProjectForCurrentUserByID(ctx, input.ProjectID); !ok {
			return false
		}
	}
	return true
}

func validateUpdateAIConversation(h *Handlers, ctx *gin.Context, _ model.User, body []byte) bool {
	var input struct {
		Title   *string `json:"title"`
		ModelID *string `json:"modelId"`
	}
	if len(body) == 0 || json.Unmarshal(body, &input) != nil || (input.Title == nil && input.ModelID == nil) {
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid_json", "invalid conversation update")
		return false
	}
	if input.Title != nil && (strings.TrimSpace(*input.Title) == "" || len([]byte(*input.Title)) > 200) {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.title_too_large", "conversation title is invalid")
		return false
	}
	return input.ModelID == nil || validateEnabledAIModel(h, ctx, *input.ModelID)
}

func validateEnabledAIModel(h *Handlers, ctx *gin.Context, modelID string) bool {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.model_required", "AI model is required")
		return false
	}
	if db := h.dbFor(ctx); db != nil {
		var selected model.AIModel
		if db.Where("id = ? AND enabled = ?", modelID, true).First(&selected).Error != nil {
			writeErrorCode(ctx, http.StatusConflict, "ai.model_not_available", "selected AI model is unavailable")
			return false
		}
	}
	return true
}

func validateTurnInput(h *Handlers, ctx *gin.Context, _ model.User, body []byte) bool {
	var input struct {
		ModelID string `json:"modelId"`
		Input   struct {
			Parts []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"input"`
	}
	if json.Unmarshal(body, &input) != nil || len(input.Input.Parts) == 0 {
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid_json", "invalid turn input")
		return false
	}
	if strings.TrimSpace(input.ModelID) == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.model_required", "AI model is required")
		return false
	}
	totalTextBytes := 0
	for index, part := range input.Input.Parts {
		text := strings.TrimSpace(part.Text)
		if part.Type != "text" || text == "" {
			writeErrorCode(ctx, http.StatusBadRequest, "ai.input_invalid", "only non-empty text parts are supported")
			return false
		}
		if index > 0 {
			totalTextBytes++
		}
		totalTextBytes += len([]byte(text))
		if totalTextBytes > aiTextInputLimitBytes {
			writeErrorCode(ctx, http.StatusRequestEntityTooLarge, "ai.input_too_large", "AI text input exceeds the limit")
			return false
		}
	}
	if strings.TrimSpace(ctx.GetHeader("Idempotency-Key")) == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required")
		return false
	}
	return true
}

func validateToolActionInput(_ *Handlers, ctx *gin.Context, _ model.User, body []byte) bool {
	var input struct {
		OperationID string         `json:"operationId"`
		Arguments   map[string]any `json:"arguments"`
		Message     string         `json:"message"`
	}
	if json.Unmarshal(body, &input) != nil || !aiToolActionOperationID.MatchString(input.OperationID) {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.input_invalid", "invalid AI tool action input")
		return false
	}
	message := strings.TrimSpace(input.Message)
	if message == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.input_invalid", "invalid AI tool action input")
		return false
	}
	if len([]byte(message)) > aiTextInputLimitBytes {
		writeErrorCode(ctx, http.StatusRequestEntityTooLarge, "ai.input_too_large", "AI text input exceeds the limit")
		return false
	}
	if input.Arguments == nil {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.input_invalid", "tool action arguments are required")
		return false
	}
	if strings.TrimSpace(ctx.GetHeader("Idempotency-Key")) == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required")
		return false
	}
	return true
}

func validateRunInput(_ *Handlers, ctx *gin.Context, _ model.User, body []byte) bool {
	var input struct {
		Text            string `json:"text"`
		ExpectedVersion *int64 `json:"expectedVersion"`
	}
	if json.Unmarshal(body, &input) != nil || input.ExpectedVersion == nil || strings.TrimSpace(input.Text) == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.input_invalid", "invalid AI run input")
		return false
	}
	if len([]byte(strings.TrimSpace(input.Text))) > aiTextInputLimitBytes {
		writeErrorCode(ctx, http.StatusRequestEntityTooLarge, "ai.input_too_large", "AI text input exceeds the limit")
		return false
	}
	return true
}

func projectIDFromAIRequest(ctx *gin.Context, body []byte) string {
	if value := strings.TrimSpace(ctx.Query("projectId")); value != "" {
		return value
	}
	var value map[string]any
	if json.Unmarshal(body, &value) != nil {
		return ""
	}
	if projectID, ok := value["projectId"].(string); ok {
		return strings.TrimSpace(projectID)
	}
	if pageContext, ok := value["pageContext"].(map[string]any); ok {
		if projectID, ok := pageContext["projectId"].(string); ok {
			return strings.TrimSpace(projectID)
		}
	}
	return ""
}

func normalizedAILocale(locale string) string {
	if strings.TrimSpace(locale) == "" {
		return "zh-CN"
	}
	return strings.TrimSpace(locale)
}

func cloneAIQuery(query url.Values) url.Values {
	result := make(url.Values, len(query))
	for key, values := range query {
		result[key] = append([]string(nil), values...)
	}
	return result
}

func expandAIInternalPath(path string, ctx *gin.Context) string {
	for _, parameter := range []string{"conversationId", "turnId", "runId", "toolCallId", "actionId", "operationId"} {
		path = strings.ReplaceAll(path, ":"+parameter, url.PathEscape(ctx.Param(parameter)))
	}
	return path
}

func aiRouteFor(ctx *gin.Context) (aiProxyRoute, bool) {
	key := ctx.Request.Method + " " + ctx.FullPath()
	route, ok := aiProxyRoutes[key]
	return route, ok
}

var aiProxyRoutes = map[string]aiProxyRoute{
	"GET /api/v1/ai/conversations":                               {method: "GET", internal: "/internal/v1/conversations"},
	"POST /api/v1/ai/conversations":                              {method: "POST", internal: "/internal/v1/conversations", status: http.StatusCreated, validate: validateCreateAIConversation},
	"GET /api/v1/ai/conversations/:conversationId":               {method: "GET", internal: "/internal/v1/conversations/:conversationId"},
	"PATCH /api/v1/ai/conversations/:conversationId":             {method: "PATCH", internal: "/internal/v1/conversations/:conversationId", validate: validateUpdateAIConversation},
	"DELETE /api/v1/ai/conversations/:conversationId":            {method: "DELETE", internal: "/internal/v1/conversations/:conversationId"},
	"GET /api/v1/ai/conversations/:conversationId/timeline":      {method: "GET", internal: "/internal/v1/conversations/:conversationId/timeline"},
	"POST /api/v1/ai/conversations/:conversationId/turns":        {method: "POST", internal: "/internal/v1/conversations/:conversationId/turns", status: http.StatusAccepted, validate: validateTurnInput},
	"POST /api/v1/ai/conversations/:conversationId/tool-actions": {method: "POST", internal: "/internal/v1/conversations/:conversationId/tool-actions", status: http.StatusAccepted, validate: validateToolActionInput},
	"GET /api/v1/ai/turns/:turnId/runs":                          {method: "GET", internal: "/internal/v1/turns/:turnId/runs"},
	"POST /api/v1/ai/turns/:turnId/runs":                         {method: "POST", internal: "/internal/v1/turns/:turnId/runs", status: http.StatusAccepted},
	"GET /api/v1/ai/runs/:runId":                                 {method: "GET", internal: "/internal/v1/runs/:runId"},
	"GET /api/v1/ai/runs/:runId/events":                          {method: "GET", internal: "/internal/v1/runs/:runId/events", stream: true},
	"POST /api/v1/ai/runs/:runId/cancel":                         {method: "POST", internal: "/internal/v1/runs/:runId/cancel", status: http.StatusAccepted},
	"POST /api/v1/ai/runs/:runId/input":                          {method: "POST", internal: "/internal/v1/runs/:runId/input", status: http.StatusAccepted, validate: validateRunInput},
	"POST /api/v1/ai/runs/:runId/approvals/:toolCallId/decision": {method: "POST", internal: "/internal/v1/runs/:runId/approvals/:toolCallId/decision", status: http.StatusAccepted},
}
