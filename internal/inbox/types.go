package inbox

import (
	"context"
	"errors"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
)

const (
	CategoryAction   = "action"
	CategoryProject  = "project"
	CategoryBilling  = "billing"
	CategorySecurity = "security"
	CategoryDelivery = "delivery"
	CategorySystem   = "system"

	PriorityLow      = "low"
	PriorityNormal   = "normal"
	PriorityHigh     = "high"
	PriorityCritical = "critical"

	ActionRequestStatusPending    = "pending"
	ActionRequestStatusProcessing = "processing"
	ActionRequestStatusCompleted  = "completed"
	ActionRequestStatusRejected   = "rejected"
	ActionRequestStatusCancelled  = "cancelled"
	ActionRequestStatusExpired    = "expired"
	ActionRequestStatusFailed     = "failed"

	ActionRequestTypeBillingOwnerTransfer = "project.billing_owner_transfer"
	ActionRequestTypeRuntimeSelfHeal      = "runtime.self_heal"
)

var (
	ErrInvalidInput            = errors.New("inbox input is invalid")
	ErrInvalidDeepLink         = errors.New("inbox deep link is invalid")
	ErrNotFound                = errors.New("inbox resource not found")
	ErrActionRequestStale      = errors.New("inbox action request is stale")
	ErrActionRequestExpired    = errors.New("inbox action request has expired")
	ErrSelfHealSessionRequired = errors.New("a browser session is required to confirm runtime self-healing")
	ErrSelfHealForbidden       = errors.New("runtime self-heal action is forbidden")
	ErrSelfHealStateDrifted    = errors.New("runtime self-heal target state has drifted")
)

type decisionMetaKey struct{}

type DecisionMeta struct {
	RequestCarrier any
	PreviewLock    string
}

func WithDecisionMeta(ctx context.Context, meta DecisionMeta) context.Context {
	return context.WithValue(ctx, decisionMetaKey{}, meta)
}

func DecisionMetaFromContext(ctx context.Context) (DecisionMeta, bool) {
	meta, ok := ctx.Value(decisionMetaKey{}).(DecisionMeta)
	return meta, ok
}

type PublishInput struct {
	RecipientUserID    string
	Type               string
	Category           string
	Priority           string
	ActorID            string
	ProjectID          string
	ResourceType       string
	ResourceID         string
	TitleKey           string
	ContentKey         string
	Params             map[string]any
	ActionRequestID    string
	ClearActionRequest bool
	DeepLink           string
	GroupKey           string
	DedupKey           string
	ExpiresAt          *time.Time
}

type CreateActionRequestInput struct {
	Type            string
	RequesterUserID string
	RecipientUserID string
	ProjectID       string
	ResourceType    string
	ResourceID      string
	Payload         map[string]any
	ExpiresAt       *time.Time
}

type ListInput struct {
	UserID    string
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
	Filter    string
	Category  string
}

type ListResult struct {
	Items      []model.InboxMessage
	Page       int
	PageSize   int
	SortBy     string
	SortOrder  string
	Total      int64
	TotalPages int
}
