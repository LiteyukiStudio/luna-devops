package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

const aiPlatformResponseLimit = 1024 * 1024

type aiPlatformActorContextKey struct{}

type aiPlatformActor struct {
	UserID       string
	SessionID    string
	MFAPurpose   string
	MFAAssertion string
}

type aiPlatformDispatchResult struct {
	Status    int
	Body      any
	RequestID string
}

func (h *Handlers) dispatchAIPlatformOperation(
	parent *gin.Context,
	claims aiagent.DelegationClaims,
	operation aitool.OpenAPIOperation,
	arguments map[string]any,
) (aiPlatformDispatchResult, error) {
	if h.platformRouter == nil {
		return aiPlatformDispatchResult{}, fmt.Errorf("platform router is unavailable")
	}
	target, body, err := buildAIPlatformRequest(operation, arguments)
	if err != nil {
		return aiPlatformDispatchResult{}, err
	}
	request, err := http.NewRequestWithContext(
		context.WithValue(parent.Request.Context(), aiPlatformActorContextKey{}, aiPlatformActor{
			UserID: claims.UserID, SessionID: claims.SessionID,
			MFAPurpose: claims.MFAPurpose, MFAAssertion: claims.MFAAssertion,
		}),
		operation.Method,
		target,
		body,
	)
	if err != nil {
		return aiPlatformDispatchResult{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Language", requestLanguage(parent))
	request.Header.Set("X-Luna-AI-Run-ID", claims.RunID)
	request.Header.Set("X-Luna-AI-Tool-Call-ID", claims.ToolCallID)
	request.Header.Set("Idempotency-Key", claims.ToolCallID+":"+claims.ArgumentsHash)
	if operation.RequestBody {
		request.Header.Set("Content-Type", operation.RequestType)
	}

	recorder := httptest.NewRecorder()
	h.platformRouter.ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, aiPlatformResponseLimit+1))
	if err != nil {
		return aiPlatformDispatchResult{}, err
	}
	if len(payload) > aiPlatformResponseLimit {
		return aiPlatformDispatchResult{}, fmt.Errorf("platform response exceeds Agent limit")
	}
	var result any
	if len(bytes.TrimSpace(payload)) > 0 {
		if json.Unmarshal(payload, &result) != nil {
			result = string(payload)
		}
	}
	return aiPlatformDispatchResult{
		Status: response.StatusCode, Body: result,
		RequestID: response.Header.Get("X-Request-ID"),
	}, nil
}

func buildAIPlatformRequest(operation aitool.OpenAPIOperation, arguments map[string]any) (string, io.Reader, error) {
	allowed := map[string]struct{}{}
	targetPath := operation.Path
	query := url.Values{}
	for _, parameter := range operation.Parameters {
		allowed[parameter.Name] = struct{}{}
		value, exists := arguments[parameter.Name]
		if parameter.Required && (!exists || value == nil || strings.TrimSpace(fmt.Sprint(value)) == "") {
			return "", nil, fmt.Errorf("required parameter %q is missing", parameter.Name)
		}
		if !exists || value == nil {
			continue
		}
		switch parameter.In {
		case "path":
			targetPath = strings.ReplaceAll(targetPath, "{"+parameter.Name+"}", url.PathEscape(fmt.Sprint(value)))
		case "query":
			appendAIQueryValue(query, parameter.Name, value)
		}
	}
	var body io.Reader
	if operation.RequestBody {
		allowed["body"] = struct{}{}
		value, exists := arguments["body"]
		if operation.RequestRequired && (!exists || value == nil) {
			return "", nil, fmt.Errorf("required request body is missing")
		}
		if exists && value != nil {
			encoded, err := json.Marshal(value)
			if err != nil {
				return "", nil, fmt.Errorf("encode request body: %w", err)
			}
			body = bytes.NewReader(encoded)
		}
	}
	for name := range arguments {
		if _, ok := allowed[name]; !ok {
			return "", nil, fmt.Errorf("unknown argument %q", name)
		}
	}
	if strings.Contains(targetPath, "{") {
		return "", nil, fmt.Errorf("platform path parameters are incomplete")
	}
	if encoded := query.Encode(); encoded != "" {
		targetPath += "?" + encoded
	}
	return targetPath, body, nil
}

func appendAIQueryValue(query url.Values, name string, value any) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			query.Add(name, fmt.Sprint(item))
		}
	case []string:
		for _, item := range typed {
			query.Add(name, item)
		}
	case bool:
		query.Add(name, strconv.FormatBool(typed))
	default:
		query.Add(name, fmt.Sprint(value))
	}
}

func (h *Handlers) currentAIPlatformUser(ctx *gin.Context) (model.User, bool) {
	actor, ok := ctx.Request.Context().Value(aiPlatformActorContextKey{}).(aiPlatformActor)
	if !ok || actor.UserID == "" || actor.SessionID == "" {
		return model.User{}, false
	}
	now := time.Now()
	var user model.User
	if h.dbFor(ctx).First(&user, "id = ? and disabled = ?", actor.UserID, false).Error != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
		return model.User{}, true
	}
	var session model.UserSession
	if h.dbFor(ctx).First(&session, "id = ? and user_id = ? and expires_at > ?", actor.SessionID, actor.UserID, now).Error != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		return model.User{}, true
	}
	if actor.MFAPurpose != "" {
		var assertion model.StepUpAssertion
		if actor.MFAAssertion == "" || h.dbFor(ctx).First(
			&assertion,
			"id = ? and user_id = ? and session_id = ? and purpose = ? and idle_expires_at > ? and absolute_expires_at > ?",
			actor.MFAAssertion, actor.UserID, actor.SessionID, actor.MFAPurpose, now, now,
		).Error != nil || !stepUpAssertionActive(assertion, now) {
			writeErrorCode(ctx, http.StatusForbidden, "mfa.assertion_invalid", "step-up assertion is invalid or expired")
			return model.User{}, true
		}
		ctx.Set(stepUpPurposeContextKey, actor.MFAPurpose)
	}
	ctx.Set(currentUserContextKey, user)
	return user, true
}
