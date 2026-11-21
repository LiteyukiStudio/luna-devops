package model

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	DefaultDeliveryDiagnosisCooldownSeconds   = 300
	DefaultDeliveryDiagnosisDailyLimit        = 20
	DefaultRuntimeDiagnosisCooldownSeconds    = 300
	DefaultRuntimeDiagnosisDailyLimit         = 20
	DefaultRuntimeMaxConsecutiveAutoHeals     = 2
	DefaultRuntimeCircuitBreakerWindowSeconds = 1800

	AIDeliverySubjectBuildRun = "build_run"
	AIDeliverySubjectRelease  = "release"

	AIDiagnosisStatusRunning   = "running"
	AIDiagnosisStatusSucceeded = "succeeded"
	AIDiagnosisStatusFailed    = "failed"

	AISelfHealModeSuggestOnly = "suggest_only"
	AISelfHealModeAutoL1      = "auto_l1"
	AISelfHealModeAutoL2      = "auto_l2"

	AIAutoHealStatusNone          = "none"
	AIAutoHealStatusSucceeded     = "succeeded"
	AIAutoHealStatusFailed        = "failed"
	AIAutoHealStatusCircuitBroken = "circuit_broken"
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

var allowedAIRuntimeHealTools = map[string]struct{}{
	"delivery_restart_target":        {},
	"delivery_rollback_target":       {},
	"delivery_scale_target":          {},
	"delivery_update_target_runtime": {},
	"delivery_deploy_target":         {},
}

func IsAllowedAIRemediationTool(toolName string) bool {
	_, ok := allowedAIRemediationTools[strings.TrimSpace(toolName)]
	return ok
}

func IsAllowedAIRuntimeHealTool(toolName string) bool {
	_, ok := allowedAIRuntimeHealTools[strings.TrimSpace(toolName)]
	return ok
}

func IsValidAISelfHealMode(mode string) bool {
	switch strings.TrimSpace(mode) {
	case AISelfHealModeSuggestOnly, AISelfHealModeAutoL1, AISelfHealModeAutoL2:
		return true
	default:
		return false
	}
}

func SelfHealModeRank(mode string) int {
	switch strings.TrimSpace(mode) {
	case AISelfHealModeAutoL2:
		return 2
	case AISelfHealModeAutoL1:
		return 1
	default:
		return 0
	}
}

type ProjectAIAutomationConfig struct {
	ProjectID                   string    `gorm:"primaryKey;column:project_id" json:"projectId"`
	DeliveryDiagnosisEnabled    bool      `gorm:"column:delivery_diagnosis_enabled;not null;default:false" json:"deliveryDiagnosisEnabled"`
	DeliveryBillingUserID       string    `gorm:"column:delivery_billing_user_id;not null;default:''" json:"deliveryBillingUserId"`
	DeliveryCooldownSeconds     int       `gorm:"column:delivery_cooldown_seconds;not null;default:300" json:"deliveryCooldownSeconds"`
	DeliveryDailyLimit          int       `gorm:"column:delivery_daily_limit;not null;default:20" json:"deliveryDailyLimit"`
	RuntimeDiagnosisEnabled     bool      `gorm:"column:runtime_diagnosis_enabled;not null;default:false" json:"runtimeDiagnosisEnabled"`
	RuntimeBillingUserID        string    `gorm:"column:runtime_billing_user_id;not null;default:''" json:"runtimeBillingUserId"`
	RuntimeCooldownSeconds      int       `gorm:"column:runtime_cooldown_seconds;not null;default:300" json:"runtimeCooldownSeconds"`
	RuntimeDailyLimit           int       `gorm:"column:runtime_daily_limit;not null;default:20" json:"runtimeDailyLimit"`
	SelfHealMode                string    `gorm:"column:self_heal_mode;not null;default:'suggest_only'" json:"selfHealMode"`
	StageSelfHealModesJSON      string    `gorm:"column:stage_self_heal_modes_json;not null;default:'{}'" json:"-"`
	AllowedAutoHealToolsJSON    string    `gorm:"column:allowed_auto_heal_tools_json;not null;default:'[]'" json:"-"`
	MaxConsecutiveAutoHeals     int       `gorm:"column:max_consecutive_auto_heals;not null;default:2" json:"maxConsecutiveAutoHeals"`
	CircuitBreakerWindowSeconds int       `gorm:"column:circuit_breaker_window_seconds;not null;default:1800" json:"circuitBreakerWindowSeconds"`
	UpdatedBy                   string    `gorm:"column:updated_by;not null;default:''" json:"updatedBy"`
	CreatedAt                   time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt                   time.Time `gorm:"column:updated_at" json:"updatedAt"`
}

func (ProjectAIAutomationConfig) TableName() string {
	return "project_ai_automation_configs"
}

func DefaultProjectAIAutomationConfig(projectID string) ProjectAIAutomationConfig {
	return ProjectAIAutomationConfig{
		ProjectID:                   strings.TrimSpace(projectID),
		DeliveryDiagnosisEnabled:    false,
		DeliveryBillingUserID:       "",
		DeliveryCooldownSeconds:     DefaultDeliveryDiagnosisCooldownSeconds,
		DeliveryDailyLimit:          DefaultDeliveryDiagnosisDailyLimit,
		RuntimeDiagnosisEnabled:     false,
		RuntimeBillingUserID:        "",
		RuntimeCooldownSeconds:      DefaultRuntimeDiagnosisCooldownSeconds,
		RuntimeDailyLimit:           DefaultRuntimeDiagnosisDailyLimit,
		SelfHealMode:                AISelfHealModeSuggestOnly,
		StageSelfHealModesJSON:      "{}",
		AllowedAutoHealToolsJSON:    "[]",
		MaxConsecutiveAutoHeals:     DefaultRuntimeMaxConsecutiveAutoHeals,
		CircuitBreakerWindowSeconds: DefaultRuntimeCircuitBreakerWindowSeconds,
	}
}

func (c ProjectAIAutomationConfig) StageSelfHealModes() map[string]string {
	out := map[string]string{}
	if raw := strings.TrimSpace(c.StageSelfHealModesJSON); raw != "" {
		var decoded map[string]string
		if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
			for k, v := range decoded {
				stage := strings.ToLower(strings.TrimSpace(k))
				mode := strings.TrimSpace(v)
				if stage != "" && IsValidAISelfHealMode(mode) {
					out[stage] = mode
				}
			}
		}
	}
	return out
}

func (c ProjectAIAutomationConfig) EffectiveSelfHealModeForStage(stage string) string {
	normalizedStage := strings.ToLower(strings.TrimSpace(stage))
	resolved := AISelfHealModeSuggestOnly
	if IsValidAISelfHealMode(c.SelfHealMode) {
		resolved = strings.TrimSpace(c.SelfHealMode)
	}
	if normalizedStage != "" {
		if override, ok := c.StageSelfHealModes()[normalizedStage]; ok && IsValidAISelfHealMode(override) {
			resolved = override
		}
	}
	if (normalizedStage == "prod" || normalizedStage == "production") && resolved == AISelfHealModeAutoL1 {
		return AISelfHealModeSuggestOnly
	}
	return resolved
}

func (c ProjectAIAutomationConfig) AllowedAutoHealTools() []string {
	out := []string{}
	if raw := strings.TrimSpace(c.AllowedAutoHealToolsJSON); raw != "" {
		var decoded []string
		if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
			seen := map[string]struct{}{}
			for _, item := range decoded {
				name := strings.TrimSpace(item)
				if IsAllowedAIRuntimeHealTool(name) {
					if _, exists := seen[name]; !exists {
						seen[name] = struct{}{}
						out = append(out, name)
					}
				}
			}
		}
	}
	return out
}

func (c ProjectAIAutomationConfig) AllowsAutoHealTool(toolName string) bool {
	name := strings.TrimSpace(toolName)
	if !IsAllowedAIRuntimeHealTool(name) {
		return false
	}
	whitelist := c.AllowedAutoHealTools()
	if len(whitelist) == 0 {
		return true
	}
	for _, allowed := range whitelist {
		if allowed == name {
			return true
		}
	}
	return false
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

type AIRuntimeIncidentDiagnosis struct {
	ID                    string     `gorm:"primaryKey;column:id"`
	ProjectID             string     `gorm:"column:project_id;not null"`
	ApplicationID         string     `gorm:"column:application_id;not null;default:''"`
	DeploymentTargetID    string     `gorm:"column:deployment_target_id;not null;default:''"`
	IncidentID            string     `gorm:"column:incident_id;not null"`
	BilledUserID          string     `gorm:"column:billed_user_id;not null;default:''"`
	AIRunID               string     `gorm:"column:ai_run_id;not null;default:''"`
	Status                string     `gorm:"column:status;not null"`
	AnomalyType           string     `gorm:"column:anomaly_type;not null;default:''"`
	RootCauseSummary      string     `gorm:"column:root_cause_summary;not null;default:''"`
	Confidence            string     `gorm:"column:confidence;not null;default:'medium'"`
	SuggestionsJSON       string     `gorm:"column:suggestions_json;not null;default:'[]'"`
	ProposedActionJSON    string     `gorm:"column:proposed_action_json;not null;default:''"`
	AutoHealMode          string     `gorm:"column:auto_heal_mode;not null;default:'suggest_only'"`
	AutoHealStatus        string     `gorm:"column:auto_heal_status;not null;default:'none'"`
	AutoHealSummary       string     `gorm:"column:auto_heal_summary;not null;default:''"`
	CircuitBreakerTripped bool       `gorm:"column:circuit_breaker_tripped;not null;default:false"`
	ErrorCode             string     `gorm:"column:error_code;not null;default:''"`
	InboxSyncedAt         *time.Time `gorm:"column:inbox_synced_at"`
	CreatedAt             time.Time  `gorm:"column:created_at"`
	FinishedAt            *time.Time `gorm:"column:finished_at"`
	UpdatedAt             time.Time  `gorm:"column:updated_at"`
}

func (AIRuntimeIncidentDiagnosis) TableName() string {
	return "ai_runtime_incident_diagnoses"
}
