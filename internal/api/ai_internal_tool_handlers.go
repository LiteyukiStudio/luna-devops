package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
)

type aiToolPolicy struct {
	OperationID      string
	Scopes           []string
	ProjectAction    authz.Action
	Risk             string
	ApprovalRequired bool
	MFAPurpose       string
}

var aiToolPolicies = buildAIToolPolicies()

func buildAIToolPolicies() map[string]aiToolPolicy {
	policies := map[string]aiToolPolicy{
		"webSearch":               {OperationID: "webSearch", Scopes: []string{"web:read"}, Risk: "read"},
		"fetchWebPage":            {OperationID: "fetchWebPage", Scopes: []string{"web:read"}, Risk: "read"},
		"listGatewayCertificates": {OperationID: "listGatewayCertificates", Scopes: []string{"gateway:read"}, ProjectAction: authz.ActionGatewayRead, Risk: "read"},
		"listRuntimeEvents":       {OperationID: "listRuntimeEvents", Scopes: []string{"event:read"}, ProjectAction: authz.ActionProjectRead, Risk: "read"},
		// getAppTemplate 是手写 service 操作，无独立 OpenAPI 路由，需手动注册策略
		"getAppTemplate": {OperationID: "getAppTemplate", Scopes: []string{"application:read"}, Risk: "read"},
	}
	operations, err := aitool.PlatformCatalog()
	if err != nil {
		panic("load Agent OpenAPI catalog: " + err.Error())
	}
	for _, operation := range operations {
		policies[operation.OperationID] = aiToolPolicy{
			OperationID: operation.OperationID,
			Scopes:      append([]string(nil), operation.RequiredScopes...),
			Risk:        operation.Risk, ApprovalRequired: operation.Approval == "always",
			MFAPurpose: operation.StepUpPurpose,
		}
	}
	return policies
}

type aiDelegationExchangeInput struct {
	RunActorGrant     string   `json:"runActorGrant"`
	RunID             string   `json:"runId"`
	ToolCallID        string   `json:"toolCallId"`
	OperationID       string   `json:"operationId"`
	RequestedScopes   []string `json:"requestedScopes"`
	ArgumentsHash     string   `json:"argumentsHash"`
	InputMode         string   `json:"inputMode"`
	ApprovalGranted   bool     `json:"approvalGranted"`
	MFAPurpose        string   `json:"mfaPurpose"`
	StepUpAssertionID string   `json:"stepUpAssertionId"`
}

func (h *Handlers) ExchangeAIDelegation(ctx *gin.Context) {
	if !requireAIAgentService(ctx) {
		return
	}
	var input aiDelegationExchangeInput
	if !bindJSON(ctx, &input) {
		return
	}
	policy, ok := aiToolPolicies[input.OperationID]
	if !ok {
		writeErrorCode(ctx, http.StatusForbidden, "ai.tool_not_allowed", "operation is not registered")
		return
	}
	if strings.TrimSpace(input.RunID) == "" || strings.TrimSpace(input.ToolCallID) == "" ||
		!validAIArgumentsHash(input.ArgumentsHash) {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.delegation_request_invalid", "delegation binding is incomplete")
		return
	}
	highRisk := policy.Risk == "sensitive" || policy.Risk == "destructive"
	if (highRisk || policy.ApprovalRequired) && !input.ApprovalGranted {
		writeErrorCode(ctx, http.StatusPreconditionRequired, "ai.approval_required", "high-risk tool requires bound approval")
		return
	}
	effectiveMFAPurpose := policy.MFAPurpose
	if input.MFAPurpose != "" {
		requestedPurpose := normalizeStepUpPurpose(input.MFAPurpose)
		if requestedPurpose == "" || (effectiveMFAPurpose != "" && requestedPurpose != effectiveMFAPurpose) {
			writeErrorCode(ctx, http.StatusPreconditionRequired, "mfa_required", "high-risk tool requires step-up verification")
			return
		}
		effectiveMFAPurpose = requestedPurpose
	}
	if policy.MFAPurpose != "" && effectiveMFAPurpose != policy.MFAPurpose {
		writeErrorCode(ctx, http.StatusPreconditionRequired, "mfa_required", "high-risk tool requires step-up verification")
		return
	}
	if input.StepUpAssertionID != "" && effectiveMFAPurpose == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.delegation_request_invalid", "step-up purpose is required with an assertion")
		return
	}
	if !sameStringSet(input.RequestedScopes, policy.Scopes) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.delegation_scope_invalid", "requested scopes do not match tool policy")
		return
	}
	input.InputMode = strings.TrimSpace(input.InputMode)
	if input.InputMode == "" {
		input.InputMode = "model"
	}
	if input.InputMode != "model" && input.InputMode != "direct" {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.delegation_request_invalid", "input mode is invalid")
		return
	}
	now := time.Now()
	internalKeys, err := aiagent.LoadInternalKeys()
	if err != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.delegation_not_configured", "delegation trust is not configured")
		return
	}
	grant, err := aiagent.VerifyRunActorGrant(input.RunActorGrant, internalKeys.RunActorGrantSigningKey, now)
	if err != nil || grant.RunID != input.RunID {
		writeErrorCode(ctx, http.StatusUnauthorized, "ai.run_actor_grant_invalid", "Run Actor Grant is invalid")
		return
	}
	if !h.authorizeAIGrantActor(ctx, grant, policy) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.authorization_changed", "actor authorization is no longer valid")
		return
	}
	if effectiveMFAPurpose != "" && !h.validAIToolMFAAssertion(grant, input.StepUpAssertionID, effectiveMFAPurpose, now, ctx.Request.Context()) {
		writeErrorCode(ctx, http.StatusForbidden, "mfa.assertion_invalid", "step-up assertion is invalid for this tool")
		return
	}
	claims := aiagent.DelegationClaims{
		Audience: "luna-api-ai-tools", Purpose: "execute_registered_tool",
		RunID: grant.RunID, ToolCallID: input.ToolCallID, OperationID: input.OperationID,
		UserID: grant.UserID, SessionID: grant.SessionID,
		Scopes: append([]string(nil), policy.Scopes...), ArgumentsHash: input.ArgumentsHash,
		InputMode:  input.InputMode,
		MFAPurpose: effectiveMFAPurpose, MFAAssertion: input.StepUpAssertionID,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(),
	}
	token, err := aiagent.SignDelegationToken(claims, internalKeys.DelegationTokenSigningKey)
	if err != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.delegation_not_configured", "delegation signing is unavailable")
		return
	}
	h.auditWithContext(grant.UserID, "ai.delegation.exchange", input.OperationID+":"+input.ToolCallID, true, "short-lived user-bound AI tool delegation issued", ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{"accessToken": token, "tokenType": "Bearer", "expiresIn": 60, "operationId": input.OperationID})
}

func (h *Handlers) validAIToolMFAAssertion(grant aiagent.RunActorGrant, assertionID, purpose string, now time.Time, ctx context.Context) bool {
	if h.dbWithContext(ctx) == nil || strings.TrimSpace(assertionID) == "" {
		return false
	}
	var assertion model.StepUpAssertion
	return h.dbWithContext(ctx).First(
		&assertion,
		"id = ? and user_id = ? and session_id = ? and purpose = ? and idle_expires_at > ? and absolute_expires_at > ?",
		assertionID, grant.UserID, grant.SessionID, purpose, now, now,
	).Error == nil && stepUpAssertionActive(assertion, now)
}

func (h *Handlers) ExecuteAIInternalTool(ctx *gin.Context) {
	claims, ok := h.requireAIDelegation(ctx, ctx.Param("operationId"))
	if !ok {
		return
	}
	var input struct {
		ArgumentsCanonical string `json:"argumentsCanonical"`
	}
	if !bindJSON(ctx, &input) {
		return
	}
	arguments, err := decodeAICanonicalArguments(input.ArgumentsCanonical)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid", "canonical tool arguments are invalid")
		return
	}
	if claims.ArgumentsHash != "" && claims.ArgumentsHash != hashAICanonicalArguments(input.ArgumentsCanonical) {
		writeErrorCode(ctx, http.StatusConflict, "ai.approval_arguments_changed", "tool arguments differ from delegated arguments")
		return
	}
	if operation, exists := aitool.PlatformOperation(claims.OperationID); exists {
		if claims.InputMode != "direct" && aiOperationHasSensitiveArguments(operation.InputSchema, arguments) {
			writeErrorCode(ctx, http.StatusBadRequest, "ai.sensitive_input_requires_user_form", "敏感输入必须通过安全表单提交")
			return
		}
		result, err := h.dispatchAIPlatformOperation(ctx, claims, operation, arguments)
		if err != nil {
			h.auditWithContext(claims.UserID, "ai.tool.execute", claims.OperationID+":"+claims.ToolCallID, false, "request.invalid", ctx.Request.Context())
			writeErrorCode(ctx, http.StatusBadRequest, "request.invalid", "AI tool arguments are invalid")
			return
		}
		if result.RequestID != "" {
			ctx.Header("X-Platform-Request-ID", result.RequestID)
		}
		success := result.Status >= http.StatusOK && result.Status < http.StatusMultipleChoices
		h.auditWithContext(claims.UserID, "ai.tool.execute", claims.OperationID+":"+claims.ToolCallID, success, "delegated platform API operation executed", ctx.Request.Context())
		if !success {
			if result.Body == nil {
				ctx.Status(result.Status)
				return
			}
			ctx.JSON(result.Status, result.Body)
			return
		}
		readOperation := strings.EqualFold(operation.Method, http.MethodGet)
		ctx.JSON(result.Status, gin.H{
			"operationId":          claims.OperationID,
			"accepted":             true,
			"verified":             readOperation,
			"result":               result.Body,
			"platformRequestId":    result.RequestID,
			"verificationRequired": !readOperation,
		})
		return
	}
	result, code, errCode := h.executeRegisteredAITool(ctx, claims, arguments)
	if errCode != "" {
		h.auditWithContext(claims.UserID, "ai.tool.execute", claims.OperationID+":"+claims.ToolCallID, false, errCode, ctx.Request.Context())
		writeErrorCode(ctx, code, errCode, "AI tool execution denied")
		return
	}
	h.auditWithContext(claims.UserID, "ai.tool.execute", claims.OperationID+":"+claims.ToolCallID, true, "registered user-bound AI tool executed", ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{"operationId": claims.OperationID, "verified": true, "result": result})
}

func aiOperationHasSensitiveArguments(schema map[string]any, value any) bool {
	if boolSchemaValue(schema["writeOnly"]) || boolSchemaValue(schema["x-luna-sensitive"]) {
		if value == nil {
			return false
		}
		if text, ok := value.(string); ok {
			return strings.TrimSpace(text) != ""
		}
		if values, ok := value.([]any); ok {
			return len(values) > 0
		}
		if values, ok := value.(map[string]any); ok {
			return len(values) > 0
		}
		return true
	}
	if object, ok := value.(map[string]any); ok {
		properties := mapValue(schema["properties"])
		for key, item := range object {
			if property := mapValue(properties[key]); len(property) > 0 && aiOperationHasSensitiveArguments(property, item) {
				return true
			}
		}
	}
	if values, ok := value.([]any); ok {
		if items := mapValue(schema["items"]); len(items) > 0 {
			for _, item := range values {
				if aiOperationHasSensitiveArguments(items, item) {
					return true
				}
			}
		}
	}
	return false
}

func boolSchemaValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func mapValue(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func (h *Handlers) VerifyAIInternalTool(ctx *gin.Context) {
	claims, ok := h.requireAIDelegation(ctx, ctx.Param("operationId"))
	if !ok {
		return
	}
	policy := aiToolPolicies[claims.OperationID]
	ctx.JSON(http.StatusOK, gin.H{
		"operationId": claims.OperationID, "allowed": true, "risk": policy.Risk,
		"mfaPurpose": policy.MFAPurpose, "argumentsHash": claims.ArgumentsHash,
	})
}

func (h *Handlers) requireAIDelegation(ctx *gin.Context, operationID string) (aiagent.DelegationClaims, bool) {
	token := strings.TrimSpace(strings.TrimPrefix(ctx.GetHeader("Authorization"), "Bearer "))
	internalKeys, keyErr := aiagent.LoadInternalKeys()
	if keyErr != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.delegation_not_configured", "delegation trust is not configured")
		return aiagent.DelegationClaims{}, false
	}
	claims, err := aiagent.VerifyDelegationToken(token, internalKeys.DelegationTokenSigningKey, time.Now())
	if err != nil || claims.OperationID != operationID {
		writeErrorCode(ctx, http.StatusUnauthorized, "ai.delegation_invalid", "delegation token is invalid for this operation")
		return claims, false
	}
	policy, exists := aiToolPolicies[operationID]
	if !exists || !h.authorizeAIDelegatedActor(ctx, claims, policy) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.authorization_changed", "actor authorization is no longer valid")
		return claims, false
	}
	return claims, true
}

func (h *Handlers) authorizeAIGrantActor(ctx *gin.Context, grant aiagent.RunActorGrant, _ aiToolPolicy) bool {
	return h.aiTools != nil && h.aiTools.AuthorizeActor(
		ctx.Request.Context(), grant.UserID, grant.SessionID, "",
		aitool.Policy{},
	)
}

func (h *Handlers) authorizeAIDelegatedActor(ctx *gin.Context, claims aiagent.DelegationClaims, _ aiToolPolicy) bool {
	return h.aiTools != nil && h.aiTools.AuthorizeActor(
		ctx.Request.Context(), claims.UserID, claims.SessionID, "",
		aitool.Policy{},
	)
}

func (h *Handlers) executeRegisteredAITool(ctx *gin.Context, claims aiagent.DelegationClaims, arguments map[string]any) (any, int, string) {
	if h.aiTools == nil {
		return nil, http.StatusServiceUnavailable, "ai.tool_service_unavailable"
	}
	policy := aiToolPolicies[claims.OperationID]
	result, err := h.aiTools.Execute(ctx.Request.Context(), aitool.Request{
		OperationID: claims.OperationID, UserID: claims.UserID, SessionID: claims.SessionID,
		Policy: aitool.Policy{ProjectAction: policy.ProjectAction}, Arguments: arguments,
	})
	switch {
	case errors.Is(err, aitool.ErrForbidden):
		return nil, http.StatusForbidden, "ai.tool_permission_denied"
	case errors.Is(err, aitool.ErrNotFound):
		return nil, http.StatusNotFound, "resource.not_found"
	case errors.Is(err, aitool.ErrInvalidInput):
		return nil, http.StatusBadRequest, "request.invalid"
	case errors.Is(err, aitool.ErrWebTargetBlocked):
		return nil, http.StatusForbidden, "ai.web_target_blocked"
	case errors.Is(err, aitool.ErrWebContentRejected):
		return nil, http.StatusUnsupportedMediaType, "ai.web_content_rejected"
	case errors.Is(err, aitool.ErrWebRequestFailed):
		telemetry.Logger().WarnContext(ctx.Request.Context(), "AI web tool request failed",
			slog.String("event.name", "ai.tool.web_request.failed"),
			slog.String("request_id", requestID(ctx)),
			slog.String("operation", claims.OperationID),
			slog.String("error.type", telemetry.ErrorType(err)),
		)
		return nil, http.StatusBadGateway, "ai.web_request_failed"
	case errors.Is(err, aitool.ErrConflict):
		return nil, http.StatusConflict, "resource.conflict"
	case errors.Is(err, aitool.ErrStorage):
		telemetry.Logger().ErrorContext(ctx.Request.Context(), "AI tool storage failed",
			slog.String("event.name", "ai.tool.storage.failed"),
			slog.String("request_id", requestID(ctx)),
			slog.String("operation", claims.OperationID),
			slog.String("error.type", telemetry.ErrorType(err)),
		)
		return nil, http.StatusServiceUnavailable, "ai.tool_storage_unavailable"
	case err != nil:
		telemetry.Logger().ErrorContext(ctx.Request.Context(), "AI tool execution failed",
			slog.String("event.name", "ai.tool.execution.failed"),
			slog.String("request_id", requestID(ctx)),
			slog.String("operation", claims.OperationID),
			slog.String("error.type", telemetry.ErrorType(err)),
		)
		return nil, http.StatusInternalServerError, "ai.tool_execution_failed"
	default:
		return gin.H{"data": result.Value, "truncated": result.Truncated}, 0, ""
	}
}

func requireAIAgentService(ctx *gin.Context) bool {
	actual := strings.TrimSpace(strings.TrimPrefix(ctx.GetHeader("Authorization"), "Bearer "))
	internalKeys, err := aiagent.LoadInternalKeys()
	if err != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.agent_service_not_configured", "Agent service identity is not configured")
		return false
	}
	expected := internalKeys.CallbackServiceToken
	if expected == "" || len(expected) != len(actual) || subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 {
		writeErrorCode(ctx, http.StatusUnauthorized, "ai.agent_service_unauthorized", "Agent service identity is invalid")
		return false
	}
	return true
}

func decodeAICanonicalArguments(encoded string) (map[string]any, error) {
	if strings.TrimSpace(encoded) == "" || len(encoded) > 256*1024 {
		return nil, errors.New("canonical arguments are empty or too large")
	}
	var arguments map[string]any
	if err := json.Unmarshal([]byte(encoded), &arguments); err != nil || arguments == nil {
		return nil, errors.New("canonical arguments must be a JSON object")
	}
	return arguments, nil
}

func hashAICanonicalArguments(encoded string) string {
	sum := sha256.Sum256([]byte(encoded))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sameStringSet(left, right []string) bool {
	a, b := append([]string(nil), left...), append([]string(nil), right...)
	sort.Strings(a)
	sort.Strings(b)
	return strings.Join(a, "\x00") == strings.Join(b, "\x00")
}

func validAIArgumentsHash(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}
