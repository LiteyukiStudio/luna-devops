package model

import "time"

// RuntimeObservation is an immutable hourly observation used as billing input.
// It is not a cache for current runtime state; live APIs continue to read Kubernetes.
type RuntimeObservation struct {
	ID                 string    `gorm:"primaryKey" json:"id"`
	DeploymentTargetID string    `gorm:"uniqueIndex:idx_runtime_observations_target_period;index;not null" json:"deploymentTargetId"`
	PeriodStart        time.Time `gorm:"uniqueIndex:idx_runtime_observations_target_period;not null" json:"periodStart"`
	PeriodEnd          time.Time `gorm:"not null" json:"periodEnd"`
	DesiredReplicas    int32     `gorm:"not null" json:"desiredReplicas"`
	UpdatedReplicas    int32     `gorm:"not null" json:"updatedReplicas"`
	ReadyReplicas      int32     `gorm:"not null" json:"readyReplicas"`
	AvailableReplicas  int32     `gorm:"not null" json:"availableReplicas"`
	CPURequest         string    `gorm:"not null" json:"cpuRequest"`
	MemoryRequest      string    `gorm:"not null" json:"memoryRequest"`
	WorkloadCreatedAt  time.Time `gorm:"not null" json:"workloadCreatedAt"`
	Status             string    `gorm:"not null" json:"status"`
	ObservationCode    string    `gorm:"not null" json:"observationCode"`
	ObservedAt         time.Time `gorm:"not null" json:"observedAt"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}
