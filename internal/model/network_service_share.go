package model

import "time"

// NetworkServiceShare 是跨项目共享：提供方项目空间把一个部署配置开放给使用方项目空间。
// 只打通网络，不注入凭据；使用方确认（AcceptedAt）且无需批准或已批准时才生效。
type NetworkServiceShare struct {
	ID                 string     `gorm:"primaryKey" json:"id"`
	ProviderProjectID  string     `gorm:"index;not null" json:"providerProjectId"`
	DeploymentTargetID string     `gorm:"not null;uniqueIndex:idx_network_service_shares_target_consumer" json:"deploymentTargetId"`
	ConsumerProjectID  string     `gorm:"not null;uniqueIndex:idx_network_service_shares_target_consumer;index" json:"consumerProjectId"`
	ApprovalRequired   bool       `gorm:"not null;default:false" json:"approvalRequired"`
	ApprovedBy         string     `gorm:"not null;default:''" json:"-"`
	ApprovedAt         *time.Time `json:"approvedAt"`
	AcceptedBy         string     `gorm:"not null;default:''" json:"-"`
	AcceptedAt         *time.Time `json:"acceptedAt"`
	CreatedBy          string     `gorm:"not null;default:''" json:"createdBy"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

// Active 表示共享已被使用方确认，且无需批准或已获批准。
func (s NetworkServiceShare) Active() bool {
	return s.AcceptedAt != nil && (!s.ApprovalRequired || s.ApprovedAt != nil)
}
