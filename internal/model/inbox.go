package model

import "time"

// InboxMessage is a user-scoped inbox projection. Read and archive state are
// independent from any linked action request state.
type InboxMessage struct {
	ID              string     `gorm:"primaryKey" json:"id"`
	RecipientUserID string     `gorm:"index;not null" json:"recipientUserId"`
	Type            string     `gorm:"index;not null" json:"type"`
	Category        string     `gorm:"index;not null" json:"category"`
	Priority        string     `gorm:"index;not null" json:"priority"`
	ActorID         string     `gorm:"not null;default:''" json:"actorId"`
	ProjectID       string     `gorm:"index;not null;default:''" json:"projectId"`
	ResourceType    string     `gorm:"not null;default:''" json:"resourceType"`
	ResourceID      string     `gorm:"not null;default:''" json:"resourceId"`
	TitleKey        string     `gorm:"not null" json:"titleKey"`
	ContentKey      string     `gorm:"not null" json:"contentKey"`
	ParamsJSON      string     `gorm:"type:jsonb;not null;default:'{}'" json:"paramsJson"`
	ActionRequestID string     `gorm:"index;not null;default:''" json:"actionRequestId"`
	DeepLink        string     `gorm:"not null;default:''" json:"deepLink"`
	GroupKey        string     `gorm:"not null;default:''" json:"groupKey"`
	DedupKey        *string    `gorm:"uniqueIndex:idx_inbox_messages_dedup_key,where:dedup_key IS NOT NULL" json:"-"`
	ReadAt          *time.Time `json:"readAt"`
	ArchivedAt      *time.Time `json:"archivedAt"`
	ExpiresAt       *time.Time `json:"expiresAt"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

// InboxActionRequest represents a typed business decision. Payload is kept as
// structured data and never interpreted as an arbitrary HTTP request.
type InboxActionRequest struct {
	ID              string     `gorm:"primaryKey" json:"id"`
	Type            string     `gorm:"index;not null" json:"type"`
	RequesterUserID string     `gorm:"index;not null" json:"requesterUserId"`
	RecipientUserID string     `gorm:"index;not null" json:"recipientUserId"`
	ProjectID       string     `gorm:"index;not null;default:''" json:"projectId"`
	ResourceType    string     `gorm:"not null;default:''" json:"resourceType"`
	ResourceID      string     `gorm:"not null;default:''" json:"resourceId"`
	PayloadJSON     string     `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
	Status          string     `gorm:"index;not null;default:pending" json:"status"`
	RowVersion      int64      `gorm:"not null;default:1" json:"rowVersion"`
	ExpiresAt       *time.Time `json:"expiresAt"`
	RespondedAt     *time.Time `json:"respondedAt"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}
