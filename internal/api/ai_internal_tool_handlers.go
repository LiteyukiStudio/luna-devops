package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
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

var aiToolPolicies = map[string]aiToolPolicy{
	"getDashboard":               {OperationID: "getDashboard", Scopes: []string{"dashboard:read"}, Risk: "read"},
	"listProjects":               {OperationID: "listProjects", Scopes: []string{"project:read"}, Risk: "read"},
	"createProject":              {OperationID: "createProject", Scopes: []string{"project:write"}, Risk: "write"},
	"listPlatformEvents":         {OperationID: "listPlatformEvents", Scopes: []string{"event:read"}, ProjectAction: authz.ActionProjectRead, Risk: "read"},
	"getProject":                 {OperationID: "getProject", Scopes: []string{"project:read"}, ProjectAction: authz.ActionProjectRead, Risk: "read"},
	"listApplications":           {OperationID: "listApplications", Scopes: []string{"application:read"}, ProjectAction: authz.ActionApplicationRead, Risk: "read"},
	"listBuildRuns":              {OperationID: "listBuildRuns", Scopes: []string{"build:read"}, ProjectAction: authz.ActionBuildRead, Risk: "read"},
	"listReleases":               {OperationID: "listReleases", Scopes: []string{"deployment:read"}, ProjectAction: authz.ActionDeploymentRead, Risk: "read"},
	"listRuntimeClusters":        {OperationID: "listRuntimeClusters", Scopes: []string{"cluster:read"}, ProjectAction: authz.ActionClusterRead, Risk: "read"},
	"listGatewayRoutes":          {OperationID: "listGatewayRoutes", Scopes: []string{"gateway:read"}, ProjectAction: authz.ActionGatewayRead, Risk: "read"},
	"listGatewayCertificates":    {OperationID: "listGatewayCertificates", Scopes: []string{"gateway:read"}, ProjectAction: authz.ActionGatewayRead, Risk: "read"},
	"listProjectHookRuns":        {OperationID: "listProjectHookRuns", Scopes: []string{"project:read"}, ProjectAction: authz.ActionProjectRead, Risk: "read"},
	"listNotificationDeliveries": {OperationID: "listNotificationDeliveries", Scopes: []string{"event:read"}, ProjectAction: authz.ActionProjectRead, Risk: "read"},
	"listRuntimeEvents":          {OperationID: "listRuntimeEvents", Scopes: []string{"event:read"}, ProjectAction: authz.ActionProjectRead, Risk: "read"},
}

type aiDelegationExchangeInput struct {
	RunActorGrant     string   `json:"runActorGrant"`
	RunID             string   `json:"runId"`
	ToolCallID        string   `json:"toolCallId"`
	OperationID       string   `json:"operationId"`
	RequestedScopes   []string `json:"requestedScopes"`
	ArgumentsHash     string   `json:"argumentsHash"`
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
	if policy.MFAPurpose != "" && input.MFAPurpose != policy.MFAPurpose {
		writeErrorCode(ctx, http.StatusPreconditionRequired, "mfa_required", "high-risk tool requires step-up verification")
		return
	}
	if !sameStringSet(input.RequestedScopes, policy.Scopes) {
		writeErrorCode(ctx, http.StatusForbidden, "ai.delegation_scope_invalid", "requested scopes do not match tool policy")
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
	if policy.MFAPurpose != "" && !h.validAIToolMFAAssertion(grant, input.StepUpAssertionID, policy.MFAPurpose, now) {
		writeErrorCode(ctx, http.StatusForbidden, "mfa.assertion_invalid", "step-up assertion is invalid for this tool")
		return
	}
	claims := aiagent.DelegationClaims{
		Audience: "luna-api-ai-tools", Purpose: "execute_registered_tool",
		RunID: grant.RunID, ToolCallID: input.ToolCallID, OperationID: input.OperationID,
		UserID: grant.UserID, SessionID: grant.SessionID,
		Scopes: append([]string(nil), policy.Scopes...), ArgumentsHash: input.ArgumentsHash,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(),
	}
	token, err := aiagent.SignDelegationToken(claims, internalKeys.DelegationTokenSigningKey)
	if err != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.delegation_not_configured", "delegation signing is unavailable")
		return
	}
	h.audit(grant.UserID, "ai.delegation.exchange", input.OperationID+":"+input.ToolCallID, true, "short-lived user-bound AI tool delegation issued")
	ctx.JSON(http.StatusOK, gin.H{"accessToken": token, "tokenType": "Bearer", "expiresIn": 60, "operationId": input.OperationID})
}

func (h *Handlers) validAIToolMFAAssertion(grant aiagent.RunActorGrant, assertionID, purpose string, now time.Time) bool {
	if h.db == nil || strings.TrimSpace(assertionID) == "" {
		return false
	}
	var assertion model.StepUpAssertion
	return h.db.First(
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
		Arguments map[string]any `json:"arguments"`
	}
	if !bindJSON(ctx, &input) {
		return
	}
	if claims.ArgumentsHash != "" && claims.ArgumentsHash != hashAIArguments(input.Arguments) {
		writeErrorCode(ctx, http.StatusConflict, "ai.approval_arguments_changed", "tool arguments differ from delegated arguments")
		return
	}
	result, code, errCode := h.executeRegisteredAITool(ctx, claims, input.Arguments)
	if errCode != "" {
		h.audit(claims.UserID, "ai.tool.execute", claims.OperationID+":"+claims.ToolCallID, false, errCode)
		writeErrorCode(ctx, code, errCode, "AI tool execution denied")
		return
	}
	h.audit(claims.UserID, "ai.tool.execute", claims.OperationID+":"+claims.ToolCallID, true, "registered user-bound AI tool executed")
	ctx.JSON(http.StatusOK, gin.H{"operationId": claims.OperationID, "verified": true, "result": result})
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
	case errors.Is(err, aitool.ErrConflict):
		return nil, http.StatusConflict, "resource.conflict"
	case errors.Is(err, aitool.ErrStorage):
		log.Printf(
			"ai tool storage failure request_id=%q operation=%q tool_call=%q: %v",
			requestID(ctx), claims.OperationID, claims.ToolCallID, err,
		)
		return nil, http.StatusServiceUnavailable, "ai.tool_storage_unavailable"
	case err != nil:
		log.Printf(
			"ai tool execution failure request_id=%q operation=%q tool_call=%q: %v",
			requestID(ctx), claims.OperationID, claims.ToolCallID, err,
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

func hashAIArguments(arguments map[string]any) string {
	encoded, _ := json.Marshal(arguments)
	sum := sha256.Sum256(encoded)
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
