package model

import "time"

// NetworkEgressException 是集群级出站例外规则：允许被授权的项目空间访问平台默认拒绝的网段（如机房内网服务）。
type NetworkEgressException struct {
	ID               string                        `gorm:"primaryKey" json:"id"`
	RuntimeClusterID string                        `gorm:"index;not null" json:"runtimeClusterId"`
	Name             string                        `gorm:"not null" json:"name"`
	CIDR             string                        `gorm:"column:cidr;not null" json:"cidr"`
	Ports            string                        `gorm:"not null;default:''" json:"ports"`
	Protocol         string                        `gorm:"not null;default:tcp" json:"protocol"`
	Reason           string                        `gorm:"not null" json:"reason"`
	CreatedBy        string                        `gorm:"not null;default:''" json:"createdBy"`
	CreatedAt        time.Time                     `json:"createdAt"`
	UpdatedAt        time.Time                     `json:"updatedAt"`
	Grants           []NetworkEgressExceptionGrant `gorm:"foreignKey:ExceptionID" json:"grants"`
}

// NetworkEgressExceptionGrant 把例外授权给一个项目空间；DeploymentTargetIDs 为空表示整个项目空间。
type NetworkEgressExceptionGrant struct {
	ID                  string    `gorm:"primaryKey" json:"-"`
	ExceptionID         string    `gorm:"not null;uniqueIndex:idx_network_egress_exception_grants_exception_project" json:"-"`
	ProjectID           string    `gorm:"not null;uniqueIndex:idx_network_egress_exception_grants_exception_project;index" json:"projectId"`
	DeploymentTargetIDs string    `gorm:"not null;default:''" json:"-"`
	IncludeBuilds       bool      `gorm:"not null;default:false" json:"includeBuilds"`
	CreatedAt           time.Time `json:"-"`
}
