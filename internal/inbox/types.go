package inbox

import (
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
)

var (
	ErrInvalidInput    = errors.New("inbox input is invalid")
	ErrInvalidDeepLink = errors.New("inbox deep link is invalid")
	ErrNotFound        = errors.New("inbox resource not found")
)

type PublishInput struct {
	RecipientUserID string
	Type            string
	Category        string
	Priority        string
	ActorID         string
	ProjectID       string
	ResourceType    string
	ResourceID      string
	TitleKey        string
	ContentKey      string
	Params          map[string]any
	ActionRequestID string
	DeepLink        string
	GroupKey        string
	DedupKey        string
	ExpiresAt       *time.Time
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
