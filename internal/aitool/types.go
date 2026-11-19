package aitool

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const (
	ApprovalTierL0 = "L0"
	ApprovalTierL1 = "L1"
	ApprovalTierL2 = "L2"

	DomainDelivery  = "delivery"
	DomainBuild     = "build"
	DomainRuntime   = "runtime"
	DomainNetwork   = "network"
	DomainWorkspace = "workspace"
	DomainOps       = "ops"
	DomainAdmin     = "admin"

	EffectRead    = "read"
	EffectWrite   = "write"
	EffectAsync   = "async"
	EffectExecute = "execute"
)

// CatalogOperation is the authoritative tool metadata exported to luna-agent
// via GET /internal/v1/ai/provider-config.
type CatalogOperation struct {
	OperationID       string                  `json:"operationId"`
	Domain            string                  `json:"domain"`
	Name              string                  `json:"name"`
	Summary           string                  `json:"summary"`
	Category          string                  `json:"category"`
	Tags              []string                `json:"tags"`
	Aliases           OperationAliases        `json:"aliases"`
	Purpose           LocalizedText           `json:"purpose"`
	AvoidWhen         LocalizedText           `json:"avoidWhen"`
	Preconditions     LocalizedList           `json:"preconditions"`
	SuccessEvidence   LocalizedText           `json:"successEvidence"`
	AdminOnly         bool                    `json:"adminOnly"`
	GlobalSessionOnly bool                    `json:"globalSessionOnly,omitempty"`
	ApprovalTier      string                  `json:"approvalTier"`
	RequiresApproval  bool                    `json:"requiresApproval"`
	Idempotent        bool                    `json:"idempotent"`
	RequiredScopes    []string                `json:"requiredScopes"`
	InputSchema       map[string]any          `json:"inputSchema"`
	OutputSchema      map[string]any          `json:"outputSchema"`
	SensitivePaths    []string                `json:"sensitivePaths,omitempty"`
	Effect            string                  `json:"effect"`
	Verification      []OperationVerification `json:"verification,omitempty"`
	RetryOnUnknown    bool                    `json:"retryOnUnknown,omitempty"`
	VerificationOnly  bool                    `json:"verificationOnly,omitempty"`
}

type OperationVerification struct {
	OperationID        string         `json:"operationId"`
	Family             string         `json:"family"`
	Expect             string         `json:"expect,omitempty"`
	WhenResultHas      string         `json:"whenResultHas,omitempty"`
	WhenArgumentEquals map[string]any `json:"whenArgumentEquals,omitempty"`
	WhenResultEquals   map[string]any `json:"whenResultEquals,omitempty"`
	IDFrom             string         `json:"idFrom,omitempty"`
	ReadIDArgument     string         `json:"readIdArgument,omitempty"`
	Singleton          bool           `json:"singleton,omitempty"`
	Collection         bool           `json:"collection,omitempty"`
	CollectionPath     string         `json:"collectionPath,omitempty"`
	StaticArguments    map[string]any `json:"staticArguments,omitempty"`
	TargetField        string         `json:"targetField,omitempty"`
	StatusField        string         `json:"statusField,omitempty"`
	PendingStatuses    []string       `json:"pendingStatuses,omitempty"`
	SuccessStatuses    []string       `json:"successStatuses,omitempty"`
	FailureStatuses    []string       `json:"failureStatuses,omitempty"`
}

type OperationAliases struct {
	ZH []string `json:"zh"`
	EN []string `json:"en"`
}

type LocalizedText struct {
	ZH string `json:"zh"`
	EN string `json:"en"`
}

type LocalizedList struct {
	ZH []string `json:"zh"`
	EN []string `json:"en"`
}

func Text(zh, en string) LocalizedText {
	if en == "" {
		en = zh
	}
	return LocalizedText{ZH: zh, EN: en}
}

func List(zh, en []string) LocalizedList {
	if zh == nil {
		zh = []string{}
	}
	if en == nil {
		en = []string{}
	}
	return LocalizedList{ZH: zh, EN: en}
}

// PreviewChange represents a single field or item diff in an L2 pre-approval preview.
type PreviewChange struct {
	Field  string `json:"field"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
	Action string `json:"action,omitempty"`
}

// ApprovalPreview is returned by POST /internal/v1/ai/tools/preview and projected
// onto L2 approval cards in Web UI. Sensitive values must never be placed in Before/After;
// list secret key names in SensitiveKeys instead.
type ApprovalPreview struct {
	Summary        string          `json:"summary"`
	TargetResource string          `json:"targetResource,omitempty"`
	TargetID       string          `json:"targetId,omitempty"`
	ProjectID      string          `json:"projectId,omitempty"`
	Changes        []PreviewChange `json:"changes,omitempty"`
	SensitiveKeys  []string        `json:"sensitiveKeys,omitempty"`
	Impact         []string        `json:"impact,omitempty"`
	Warnings       []string        `json:"warnings,omitempty"`
}

// PreviewResult pairs the user-facing ApprovalPreview with an optional opaque
// LockedPayload that will be HMAC-signed into X-Luna-AI-Preview-Lock and verified
// before Execute runs.
type PreviewResult struct {
	Preview       ApprovalPreview
	LockedPayload any
}

// AmbiguousCandidate identifies one matching resource when a name locator is ambiguous.
type AmbiguousCandidate struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Identifier string `json:"identifier,omitempty"`
	Stage      string `json:"stage,omitempty"`
}

// ToolError represents a structured HTTP/domain error returned to luna-agent for
// model self-repair (R11) or terminal rejection.
type ToolError struct {
	Status     int                  `json:"-"`
	Code       string               `json:"code"`
	Message    string               `json:"message"`
	Hint       string               `json:"hint,omitempty"`
	Retryable  *bool                `json:"retryable,omitempty"`
	Candidates []AmbiguousCandidate `json:"candidates,omitempty"`
	Details    map[string]any       `json:"details,omitempty"`
	RawBody    json.RawMessage      `json:"-"`
}

func (e *ToolError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return e.Code
}

func NewError(status int, code, message string) *ToolError {
	return &ToolError{Status: status, Code: code, Message: message}
}

func NewErrorWithHint(status int, code, message, hint string) *ToolError {
	return &ToolError{Status: status, Code: code, Message: message, Hint: hint}
}

func BadRequest(code, message string) *ToolError {
	return NewError(http.StatusBadRequest, code, message)
}

func Forbidden(code, message string) *ToolError {
	nonRetryable := false
	return &ToolError{Status: http.StatusForbidden, Code: code, Message: message, Retryable: &nonRetryable}
}

func NotFound(code, message string) *ToolError {
	return NewError(http.StatusNotFound, code, message)
}

func Conflict(code, message string) *ToolError {
	return NewError(http.StatusConflict, code, message)
}

func AmbiguousResource(resourceKind, value string, candidates []AmbiguousCandidate) *ToolError {
	return &ToolError{
		Status:     http.StatusConflict,
		Code:       "ai.resource_ambiguous",
		Message:    fmt.Sprintf("multiple %s resources match %q; specify the exact id", resourceKind, value),
		Hint:       "Select the exact resource ID from candidates and retry.",
		Candidates: candidates,
	}
}
