package aitool

import (
	"context"
	"errors"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

var (
	ErrForbidden = errors.New("AI tool target is forbidden")
	ErrNotFound  = errors.New("AI tool resource was not found")
)

type Policy struct {
	ProjectRoles []string
}

type Request struct {
	OperationID string
	UserID      string
	SessionID   string
	ProjectID   string
	Arguments   map[string]any
}

type Result struct {
	Value     any
	Truncated bool
}

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) AuthorizeActor(ctx context.Context, userID, sessionID, projectID string, policy Policy) bool {
	var user model.User
	if s.db.WithContext(ctx).First(&user, "id = ? and disabled = ?", userID, false).Error != nil {
		return false
	}
	var session model.UserSession
	if s.db.WithContext(ctx).First(&session, "id = ? and user_id = ? and expires_at > ?", sessionID, userID, time.Now()).Error != nil {
		return false
	}
	if len(policy.ProjectRoles) == 0 {
		return true
	}
	if projectID == "" {
		return false
	}
	var count int64
	return s.db.WithContext(ctx).Table("project_members").
		Where("project_id = ? and user_id = ? and role in ?", projectID, userID, policy.ProjectRoles).
		Count(&count).Error == nil && count > 0
}

func (s *Service) Execute(ctx context.Context, input Request) (Result, error) {
	if requestedProject, _ := input.Arguments["projectId"].(string); requestedProject != "" && requestedProject != input.ProjectID {
		return Result{}, ErrForbidden
	}
	const limit = 20
	switch input.OperationID {
	case "getDashboard":
		var projects []map[string]any
		err := s.db.WithContext(ctx).Table("projects").Select("projects.id, projects.name, projects.identifier").
			Joins("join project_members on project_members.project_id = projects.id").
			Where("project_members.user_id = ?", input.UserID).Order("projects.updated_at desc").Limit(limit).Scan(&projects).Error
		return Result{Value: map[string]any{"projects": projects}, Truncated: len(projects) == limit}, err
	case "getProject":
		var item map[string]any
		err := s.db.WithContext(ctx).Table("projects").
			Select("id, name, identifier, description, created_at, updated_at").
			Where("id = ?", input.ProjectID).Take(&item).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Result{}, ErrNotFound
		}
		return Result{Value: item}, err
	case "listApplications":
		return s.scanProjectRows(ctx, "applications", input.ProjectID, "id, name, identifier, description, created_at, updated_at", limit)
	case "listBuildRuns":
		return s.scanProjectRows(ctx, "build_runs", input.ProjectID, "id, application_id, status, created_at, updated_at", limit)
	case "listReleases":
		return s.scanProjectRows(ctx, "releases", input.ProjectID, "id, application_id, status, created_at, updated_at", limit)
	case "listPlatformEvents":
		return s.scanProjectRows(ctx, "platform_events", input.ProjectID, "id, type, category, severity, status, resource_type, resource_id, occurred_at", limit)
	case "listGatewayRoutes":
		return s.scanProjectRowsWhere(ctx, "gateway_routes", input.ProjectID,
			"id, application_id, deployment_target_id, host, path, tls_mode, dns_status, certificate_status, status, enabled, updated_at",
			"deleted_at is null", limit)
	case "listGatewayCertificates":
		return s.scanProjectRowsWhere(ctx, "gateway_routes", input.ProjectID,
			"id, application_id, host, tls_mode, certificate_status, certificate_message, certificate_not_after, certificate_issuer_kind, certificate_issuer_name, updated_at",
			"deleted_at is null and tls_mode <> 'http-only'", limit)
	case "listProjectHookRuns":
		return s.scanProjectRows(ctx, "hook_runs", input.ProjectID,
			"id, hook_config_id, build_run_id, release_id, application_id, name, phase, status, exit_code, message, started_at, finished_at, created_at", limit)
	case "listNotificationDeliveries":
		return s.scanProjectRows(ctx, "notification_deliveries", input.ProjectID,
			"id, event_id, event_type, severity, channel_id, adapter_kind, status, attempt_count, duration_millis, error_message, queued_at, started_at, finished_at", limit)
	case "listRuntimeEvents":
		return s.scanProjectRowsWhere(ctx, "platform_events", input.ProjectID,
			"id, type, category, severity, status, application_id, deployment_target_id, resource_type, resource_id, summary_key, message, correlation_id, occurred_at",
			"(category = 'runtime' or resource_type in ('pod', 'deployment', 'statefulset', 'job', 'runtime_cluster'))", limit)
	case "listRuntimeClusters":
		var rows []map[string]any
		projectResources := s.db.WithContext(ctx).Table("scoped_resource_project_bindings").Select("resource_id").
			Where("resource_type = ? and project_id = ?", "runtime_cluster", input.ProjectID)
		err := s.db.WithContext(ctx).Table("runtime_clusters").Select("id, name, type, scope, status, created_at, updated_at").
			Where("scope = 'global' or (scope = 'user' and owner_ref = ?) or (scope = 'project' and id in (?))", input.UserID, projectResources).
			Order("created_at desc").Limit(limit).Scan(&rows).Error
		return Result{Value: map[string]any{"items": rows}, Truncated: len(rows) == limit}, err
	default:
		return Result{}, ErrForbidden
	}
}

func (s *Service) scanProjectRows(ctx context.Context, table, projectID, columns string, limit int) (Result, error) {
	return s.scanProjectRowsWhere(ctx, table, projectID, columns, "", limit)
}

func (s *Service) scanProjectRowsWhere(ctx context.Context, table, projectID, columns, condition string, limit int) (Result, error) {
	var rows []map[string]any
	query := s.db.WithContext(ctx).Table(table).Select(columns).Where("project_id = ?", projectID)
	if condition != "" {
		query = query.Where(condition)
	}
	err := query.Order("created_at desc").Limit(limit).Scan(&rows).Error
	return Result{Value: map[string]any{"items": rows}, Truncated: len(rows) == limit}, err
}
