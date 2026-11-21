package model

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	DefaultDeliveryDiagnosisCooldownSeconds = 300
	DefaultDeliveryDiagnosisDailyLimit      = 20

	AIDeliverySubjectBuildRun = "build_run"
	AIDeliverySubjectRelease  = "release"

	AIDiagnosisStatusRunning   = "running"
	AIDiagnosisStatusSucceeded = "succeeded"
	AIDiagnosisStatusFailed    = "failed"
)

var allowedAIRemediationTools = map[string]struct{}{
	"build_trigger_run":               {},
	"build_retry_run":                 {},
	"build_configure_target_pipeline": {},
	"delivery_deploy_target":          {},
	"delivery_rollback_target":        {},
	"delivery_restart_target":         {},
	"delivery_scale_target":           {},
	"delivery_update_target_runtime":  {},
}

func IsAllowedAIRemediationTool(toolName string) bool {
	_, ok := allowedAIRemediationTools[strings.TrimSpace(toolName)]
	return ok
}

type ProjectAIAutomationConfig struct {
	ProjectID                string    `gorm:"primaryKey;column:project_id" json:"projectId"`
	DeliveryDiagnosisEnabled bool      `gorm:"column:delivery_diagnosis_enabled;not null;default:false" json:"deliveryDiagnosisEnabled"`
	DeliveryBillingUserID    string    `gorm:"column:delivery_billing_user_id;not null;default:''" json:"deliveryBillingUserId"`
	DeliveryCooldownSeconds  int       `gorm:"column:delivery_cooldown_seconds;not null;default:300" json:"deliveryCooldownSeconds"`
	DeliveryDailyLimit       int       `gorm:"column:delivery_daily_limit;not null;default:20" json:"deliveryDailyLimit"`
	UpdatedBy                string    `gorm:"column:updated_by;not null;default:''" json:"updatedBy"`
	CreatedAt                time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt                time.Time `gorm:"column:updated_at" json:"updatedAt"`
}

func (ProjectAIAutomationConfig) TableName() string {
	return "project_ai_automation_configs"
}

func DefaultProjectAIAutomationConfig(projectID string) ProjectAIAutomationConfig {
	return ProjectAIAutomationConfig{
		ProjectID:                strings.TrimSpace(projectID),
		DeliveryDiagnosisEnabled: false,
		DeliveryBillingUserID:    "",
		DeliveryCooldownSeconds:  DefaultDeliveryDiagnosisCooldownSeconds,
		DeliveryDailyLimit:       DefaultDeliveryDiagnosisDailyLimit,
	}
}

type AIProposedAction struct {
	ToolName  string         `json:"toolName"`
	Arguments map[string]any `json:"arguments"`
	Summary   string         `json:"summary"`
}

type AIDeliveryDiagnosisView struct {
	ID                 string            `json:"id"`
	SubjectType        string            `json:"subjectType"`
	SubjectID          string            `json:"subjectId"`
	Status             string            `json:"status"`
	FailureCode        string            `json:"failureCode"`
	RootCauseSummary   string            `json:"rootCauseSummary"`
	Confidence         string            `json:"confidence"`
	Suggestions        []string          `json:"suggestions"`
	ProposedAction     *AIProposedAction `json:"proposedAction,omitempty"`
	ErrorCode          string            `json:"errorCode,omitempty"`
	CreatedAt          time.Time         `json:"createdAt"`
	FinishedAt         *time.Time        `json:"finishedAt,omitempty"`
	BilledUserID       string            `json:"billedUserId,omitempty"`
	DeploymentTargetID string            `json:"deploymentTargetId,omitempty"`
}

type AIDeliveryDiagnosis struct {
	ID                    string     `gorm:"primaryKey;column:id"`
	ProjectID             string     `gorm:"column:project_id;not null"`
	ApplicationID         string     `gorm:"column:application_id;not null;default:''"`
	DeploymentTargetID    string     `gorm:"column:deployment_target_id;not null;default:''"`
	SubjectType           string     `gorm:"column:subject_type;not null"`
	SubjectID             string     `gorm:"column:subject_id;not null"`
	BilledUserID          string     `gorm:"column:billed_user_id;not null;default:''"`
	AIRunID               string     `gorm:"column:ai_run_id;not null;default:''"`
	Status                string     `gorm:"column:status;not null"`
	FailureCode           string     `gorm:"column:failure_code;not null;default:''"`
	RootCauseSummary      string     `gorm:"column:root_cause_summary;not null;default:''"`
	Confidence            string     `gorm:"column:confidence;not null;default:'medium'"`
	SuggestionsJSON       string     `gorm:"column:suggestions_json;not null;default:'[]'"`
	ProposedActionJSON    string     `gorm:"column:proposed_action_json;not null;default:''"`
	ErrorCode             string     `gorm:"column:error_code;not null;default:''"`
	NotificationEmittedAt *time.Time `gorm:"column:notification_emitted_at"`
	CreatedAt             time.Time  `gorm:"column:created_at"`
	FinishedAt            *time.Time `gorm:"column:finished_at"`
	UpdatedAt             time.Time  `gorm:"column:updated_at"`
}

func (AIDeliveryDiagnosis) TableName() string {
	return "ai_delivery_diagnoses"
}

func (d AIDeliveryDiagnosis) ToView() AIDeliveryDiagnosisView {
	suggestions := []string{}
	if raw := strings.TrimSpace(d.SuggestionsJSON); raw != "" {
		_ = json.Unmarshal([]byte(raw), &suggestions)
	}
	if suggestions == nil {
		suggestions = []string{}
	}
	var action *AIProposedAction
	if raw := strings.TrimSpace(d.ProposedActionJSON); raw != "" {
		var parsed AIProposedAction
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil && strings.TrimSpace(parsed.ToolName) != "" {
			if parsed.Arguments == nil {
				parsed.Arguments = map[string]any{}
			}
			action = &parsed
		}
	}
	return AIDeliveryDiagnosisView{
		ID:                 d.ID,
		SubjectType:        d.SubjectType,
		SubjectID:          d.SubjectID,
		Status:             d.Status,
		FailureCode:        d.FailureCode,
		RootCauseSummary:   d.RootCauseSummary,
		Confidence:         d.Confidence,
		Suggestions:        suggestions,
		ProposedAction:     action,
		ErrorCode:          d.ErrorCode,
		CreatedAt:          d.CreatedAt,
		FinishedAt:         d.FinishedAt,
		BilledUserID:       d.BilledUserID,
		DeploymentTargetID: d.DeploymentTargetID,
	}
}
