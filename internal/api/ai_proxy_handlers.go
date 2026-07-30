package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

const (
	aiAssistantEnabledConfigKey = "ai.assistant.enabled"
	aiMaxInputBytes             = 32768
)

type aiProxyRoute struct {
	method   string
	internal string
	stream   bool
	status   int
	validate func(*Handlers, *gin.Context, model.User, []byte) bool
}

func (h *Handlers) GetAICapabilities(ctx *gin.Context) {
	actor, ok := h.aiActorFromSession(ctx)
	if !ok {
		return
	}
	if reason := h.aiUnavailableReason(); reason != "" {
		ctx.JSON(http.StatusOK, unavailableAICapabilities(reason))
		return
	}
	response, err := h.aiAgent.Do(ctx.Request.Context(), actor, aiagent.Request{
		Method: http.MethodGet, Path: "/internal/v1/capabilities",
	})
	if err != nil {
		ctx.JSON(http.StatusOK, unavailableAICapabilities("ai.agent_unavailable"))
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		ctx.JSON(http.StatusOK, unavailableAICapabilities("ai.agent_unavailable"))
		return
	}
	h.copyAIResponse(ctx, response, http.StatusOK)
}

func (h *Handlers) ProxyAIRequest(ctx *gin.Context) {
	route, ok := aiRouteFor(ctx)
	if !ok {
		writeErrorCode(ctx, http.StatusNotFound, "resource.not_found", "AI route not found")
		return
	}
	actor, ok := h.aiActorFromSession(ctx)
	if !ok {
		return
	}
	if reason := h.aiUnavailableReason(); reason != "" {
		writeErrorCode(ctx, http.StatusServiceUnavailable, reason, "AI assistant is unavailable")
		return
	}

	body, ok := readAIBody(ctx)
	if !ok {
		return
	}
	if len(body) > 0 && containsUntrustedAIIdentity(body) {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.actor_field_forbidden", "actor identity is derived from the current session")
		return
	}
	if route.validate != nil && !route.validate(h, ctx, model.User{ID: actor.UserID, Language: actor.Locale}, body) {
		return
	}

	actor.ProjectID = projectIDFromAIRequest(ctx, body)
	actor.RunID = strings.TrimSpace(ctx.Param("runId"))
	if ctx.FullPath() == "/api/v1/ai/conversations/:conversationId/turns" {
		var prepared bool
		body, actor, prepared = prepareAITurnGrant(ctx, actor, body)
		if !prepared {
			return
		}
	}
	if ctx.FullPath() == "/api/v1/ai/runs/:runId/mfa/:toolCallId/resume" {
		var prepared bool
		body, prepared = h.prepareAIToolMFAResume(ctx, actor, body)
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
	h.copyAIResponse(ctx, response, route.status)
}

func (h *Handlers) prepareAIToolMFAResume(ctx *gin.Context, actor aiagent.ActorContext, body []byte) ([]byte, bool) {
	var input struct {
		StepUpAssertionID string `json:"stepUpAssertionId"`
		ExpectedVersion   int    `json:"expectedVersion"`
	}
	if json.Unmarshal(body, &input) != nil || strings.TrimSpace(input.StepUpAssertionID) == "" || input.ExpectedVersion < 1 {
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid_json", "invalid MFA resume input")
		return nil, false
	}
	now := time.Now()
	var assertion model.StepUpAssertion
	if h.db == nil || h.db.First(
		&assertion,
		"id = ? and user_id = ? and session_id = ? and idle_expires_at > ? and absolute_expires_at > ?",
		input.StepUpAssertionID, actor.UserID, actor.SessionID, now, now,
	).Error != nil || !stepUpAssertionActive(assertion, now) {
		writeErrorCode(ctx, http.StatusForbidden, "mfa.assertion_invalid", "step-up assertion is invalid or expired")
		return nil, false
	}
	prepared, err := json.Marshal(gin.H{
		"stepUpAssertionId": assertion.ID,
		"purpose":           assertion.Purpose,
		"expectedVersion":   input.ExpectedVersion,
	})
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.run_resume_failed", "cannot prepare MFA resume")
		return nil, false
	}
	return prepared, true
}

func (h *Handlers) aiActorFromSession(ctx *gin.Context) (aiagent.ActorContext, bool) {
	if h.aiActorResolver != nil {
		return h.aiActorResolver(ctx)
	}
	session, ok := h.currentSessionFromCookie(ctx)
	if !ok {
		writeErrorCode(ctx, http.StatusUnauthorized, "auth.session.missing", "a browser session is required")
		return aiagent.ActorContext{}, false
	}
	var user model.User
	if h.db == nil || h.db.First(&user, "id = ? and disabled = ?", session.UserID, false).Error != nil {
		writeErrorCode(ctx, http.StatusUnauthorized, "auth.session.expired", "the browser session is invalid")
		return aiagent.ActorContext{}, false
	}
	return aiagent.ActorContext{
		UserID: user.ID, SessionID: session.ID, Locale: normalizedAILocale(user.Language),
		RequestID: id.New("req"), SessionExpiresAt: session.ExpiresAt.Unix(),
	}, true
}

func prepareAITurnGrant(ctx *gin.Context, actor aiagent.ActorContext, body []byte) ([]byte, aiagent.ActorContext, bool) {
	var input map[string]any
	if json.Unmarshal(body, &input) != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid_json", "invalid turn input")
		return nil, actor, false
	}
	now := time.Now()
	input["pageContext"] = enrichAIPageContext(input["pageContext"], actor, now)
	expiresAt := now.Add(24 * time.Hour)
	if actor.SessionExpiresAt > 0 && time.Unix(actor.SessionExpiresAt, 0).Before(expiresAt) {
		expiresAt = time.Unix(actor.SessionExpiresAt, 0)
	}
	runID := id.New("airun")
	internalKeys, err := aiagent.LoadInternalKeys()
	if err != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.delegation_not_configured", "Run delegation trust is not configured")
		return nil, actor, false
	}
	grant, err := aiagent.SignRunActorGrant(aiagent.RunActorGrant{
		Audience: "luna-ai-run-grant", Purpose: "agent_delegation_exchange",
		RunID: runID, UserID: actor.UserID, SessionID: actor.SessionID,
		OAuthGrantID: actor.OAuthGrantID,
		IssuedAt:     now.Unix(), ExpiresAt: expiresAt.Unix(),
	}, internalKeys.RunActorGrantSigningKey)
	if err != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.delegation_not_configured", "Run delegation trust is not configured")
		return nil, actor, false
	}
	input["runId"] = runID
	input["runActorGrant"] = grant
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
	if !h.aiDeploymentEnabled {
		return "ai.disabled"
	}
	if !strings.EqualFold(strings.TrimSpace(h.configs.get([]string{aiAssistantEnabledConfigKey})[aiAssistantEnabledConfigKey]), "true") {
		return "ai.disabled"
	}
	if h.aiAgent == nil {
		return "ai.agent_unavailable"
	}
	return ""
}

func unavailableAICapabilities(reason string) gin.H {
	return gin.H{
		"available": false, "reasonCode": reason,
		"features": gin.H{"streaming": false, "approvals": false, "stepUpMFA": false, "uiActions": false, "longTermMemory": false},
		"limits":   gin.H{"maxInputBytes": aiMaxInputBytes, "maxConcurrentRuns": 0},
	}
}

func (h *Handlers) copyAIResponse(ctx *gin.Context, response *aiagent.Response, fallbackStatus int) {
	status := response.StatusCode
	if status == 0 {
		status = fallbackStatus
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

func readAIBody(ctx *gin.Context) ([]byte, bool) {
	if ctx.Request.Body == nil {
		return nil, true
	}
	body, err := io.ReadAll(io.LimitReader(ctx.Request.Body, aiMaxInputBytes+1))
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid", "cannot read request body")
		return nil, false
	}
	if len(body) > aiMaxInputBytes {
		writeErrorCode(ctx, http.StatusRequestEntityTooLarge, "ai.input_too_large", "AI request body exceeds the limit")
		return nil, false
	}
	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	return body, true
}

func containsUntrustedAIIdentity(body []byte) bool {
	var value any
	if json.Unmarshal(body, &value) != nil {
		return false
	}
	var walk func(any) bool
	walk = func(current any) bool {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if strings.EqualFold(key, "userId") || strings.EqualFold(key, "sessionId") || strings.EqualFold(key, "oauthGrantId") {
					return true
				}
				if walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(value)
}

func validateCreateAIConversation(h *Handlers, ctx *gin.Context, user model.User, body []byte) bool {
	var input struct {
		ProjectID string `json:"projectId"`
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
	if input.ProjectID != "" {
		if _, ok := h.findProjectForCurrentUserByID(ctx, input.ProjectID); !ok {
			return false
		}
	}
	return true
}

func validateTurnInput(_ *Handlers, ctx *gin.Context, _ model.User, body []byte) bool {
	var input struct {
		Input struct {
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
	for _, part := range input.Input.Parts {
		if part.Type != "text" || strings.TrimSpace(part.Text) == "" {
			writeErrorCode(ctx, http.StatusBadRequest, "ai.input_invalid", "only non-empty text parts are supported")
			return false
		}
	}
	if strings.TrimSpace(ctx.GetHeader("Idempotency-Key")) == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required")
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
	for _, parameter := range []string{"conversationId", "turnId", "runId", "toolCallId"} {
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
	"PATCH /api/v1/ai/conversations/:conversationId":             {method: "PATCH", internal: "/internal/v1/conversations/:conversationId"},
	"DELETE /api/v1/ai/conversations/:conversationId":            {method: "DELETE", internal: "/internal/v1/conversations/:conversationId"},
	"GET /api/v1/ai/conversations/:conversationId/timeline":      {method: "GET", internal: "/internal/v1/conversations/:conversationId/timeline"},
	"POST /api/v1/ai/conversations/:conversationId/turns":        {method: "POST", internal: "/internal/v1/conversations/:conversationId/turns", status: http.StatusAccepted, validate: validateTurnInput},
	"GET /api/v1/ai/turns/:turnId/runs":                          {method: "GET", internal: "/internal/v1/turns/:turnId/runs"},
	"POST /api/v1/ai/turns/:turnId/runs":                         {method: "POST", internal: "/internal/v1/turns/:turnId/runs", status: http.StatusAccepted},
	"GET /api/v1/ai/runs/:runId":                                 {method: "GET", internal: "/internal/v1/runs/:runId"},
	"GET /api/v1/ai/runs/:runId/events":                          {method: "GET", internal: "/internal/v1/runs/:runId/events", stream: true},
	"POST /api/v1/ai/runs/:runId/cancel":                         {method: "POST", internal: "/internal/v1/runs/:runId/cancel", status: http.StatusAccepted},
	"POST /api/v1/ai/runs/:runId/input":                          {method: "POST", internal: "/internal/v1/runs/:runId/input", status: http.StatusAccepted},
	"POST /api/v1/ai/runs/:runId/approvals/:toolCallId/decision": {method: "POST", internal: "/internal/v1/runs/:runId/approvals/:toolCallId/decision", status: http.StatusAccepted},
	"POST /api/v1/ai/runs/:runId/mfa/:toolCallId/resume":         {method: "POST", internal: "/internal/v1/runs/:runId/mfa/:toolCallId/resume", status: http.StatusAccepted},
}
