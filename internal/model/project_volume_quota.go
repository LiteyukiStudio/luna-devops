package model

import "time"

// ProjectVolumeQuotaUsage is the transactionally maintained aggregate of
// committed and in-flight managed volume capacity for one project space.
type ProjectVolumeQuotaUsage struct {
	ProjectID     string    `gorm:"primaryKey" json:"projectId"`
	ReservedBytes int64     `gorm:"not null;default:0" json:"reservedBytes"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func (ProjectVolumeQuotaUsage) TableName() string {
	return "project_volume_quota_usage"
}

// ProjectVolumeQuotaReservation keeps committed capacity separate from a
// pending create/import/expand delta so a failed operation can release only
// the capacity that it attempted to add.
type ProjectVolumeQuotaReservation struct {
	ProjectVolumeID string    `gorm:"primaryKey" json:"projectVolumeId"`
	ProjectID       string    `gorm:"index;not null" json:"projectId"`
	CommittedBytes  int64     `gorm:"not null;default:0" json:"committedBytes"`
	PendingBytes    int64     `gorm:"not null;default:0" json:"pendingBytes"`
	UpdatedAt       time.Time `json:"updatedAt"`
}
