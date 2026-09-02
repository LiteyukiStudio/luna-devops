package model

import "time"

// RuntimeObservation is an immutable minute observation used as billing input.
// It is not a cache for current runtime state; live APIs continue to read Kubernetes.
type RuntimeObservation struct {
	ID                     string    `gorm:"primaryKey" json:"id"`
	DeploymentTargetID     string    `gorm:"uniqueIndex:idx_runtime_observations_target_period;index;not null" json:"deploymentTargetId"`
	RuntimeClusterID       string    `gorm:"index;not null" json:"runtimeClusterId"`
	ProjectID              string    `gorm:"index;not null" json:"projectId"`
	PeriodStart            time.Time `gorm:"uniqueIndex:idx_runtime_observations_target_period;not null" json:"periodStart"`
	PeriodEnd              time.Time `gorm:"not null" json:"periodEnd"`
	DesiredReplicas        int32     `gorm:"not null" json:"desiredReplicas"`
	UpdatedReplicas        int32     `gorm:"not null" json:"updatedReplicas"`
	ReadyReplicas          int32     `gorm:"not null" json:"readyReplicas"`
	AvailableReplicas      int32     `gorm:"not null" json:"availableReplicas"`
	EffectiveCPURequest    string    `gorm:"not null" json:"effectiveCpuRequest"`
	EffectiveMemoryRequest string    `gorm:"not null" json:"effectiveMemoryRequest"`
	CPUUsageMilli          int64     `gorm:"not null" json:"cpuUsageMilli"`
	MemoryUsageBytes       int64     `gorm:"not null" json:"memoryUsageBytes"`
	MetricsAvailable       bool      `gorm:"not null" json:"metricsAvailable"`
	PodCount               int       `gorm:"not null" json:"podCount"`
	ContainerCount         int       `gorm:"not null" json:"containerCount"`
	CPURequestPercent      int       `gorm:"not null" json:"cpuRequestPercent"`
	MemoryRequestPercent   int       `gorm:"not null" json:"memoryRequestPercent"`
	CPULimitPercent        int       `gorm:"not null" json:"cpuLimitPercent"`
	MemoryLimitPercent     int       `gorm:"not null" json:"memoryLimitPercent"`
	WorkloadCreatedAt      time.Time `gorm:"not null" json:"workloadCreatedAt"`
	Status                 string    `gorm:"not null" json:"status"`
	ObservationCode        string    `gorm:"not null" json:"observationCode"`
	ObservedAt             time.Time `gorm:"not null" json:"observedAt"`
	CreatedAt              time.Time `json:"createdAt"`
	UpdatedAt              time.Time `json:"updatedAt"`
}
