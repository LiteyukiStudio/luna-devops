package model

import "time"

// NetworkServiceShare 是跨项目共享：提供方项目空间把一个部署配置开放给使用方项目空间。
// 只打通网络，不注入凭据。使用方确认（AcceptedAt）后生效；提供方在部署配置当前所在集群持有出站例外时，
// 还需该集群维护者批准，批准只对 ApprovedRuntimeClusterID 指向的集群有效。
type NetworkServiceShare struct {
	ID                       string     `gorm:"primaryKey" json:"id"`
	ProviderProjectID        string     `gorm:"index;not null" json:"providerProjectId"`
	DeploymentTargetID       string     `gorm:"not null;uniqueIndex:idx_network_service_shares_target_consumer" json:"deploymentTargetId"`
	ConsumerProjectID        string     `gorm:"not null;uniqueIndex:idx_network_service_shares_target_consumer;index" json:"consumerProjectId"`
	ApprovedRuntimeClusterID string     `gorm:"column:approved_runtime_cluster_id;not null;default:''" json:"-"`
	ApprovedBy               string     `gorm:"not null;default:''" json:"-"`
	ApprovedAt               *time.Time `json:"approvedAt"`
	AcceptedBy               string     `gorm:"not null;default:''" json:"-"`
	AcceptedAt               *time.Time `json:"acceptedAt"`
	CreatedBy                string     `gorm:"not null;default:''" json:"createdBy"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}
