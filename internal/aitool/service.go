package aitool

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"gorm.io/gorm"
)

var (
	ErrForbidden    = errors.New("AI tool target is forbidden")
	ErrNotFound     = errors.New("AI tool resource was not found")
	ErrInvalidInput = errors.New("AI tool input is invalid")
	ErrConflict     = errors.New("AI tool operation conflicts with current state")
	ErrStorage      = errors.New("AI tool storage is unavailable")
)

const applicationListColumns = "id, name, identifier, icon, delete_status, created_at, updated_at"

type Policy struct {
	ProjectAction authz.Action
}

type Request struct {
	OperationID string
	UserID      string
	SessionID   string
	Policy      Policy
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
	if policy.ProjectAction == "" {
		return true
	}
	if authz.IsPlatformAdmin(user.Role) {
		return true
	}
	if projectID == "" {
		return false
	}
	var member model.ProjectMember
	if s.db.WithContext(ctx).First(&member, "project_id = ? and user_id = ?", projectID, userID).Error != nil {
		return false
	}
	return authz.ProjectRoleAllows(member.Role, policy.ProjectAction)
}

func (s *Service) Execute(ctx context.Context, input Request) (Result, error) {
	projectID, err := targetProjectID(input.Policy, input.Arguments)
	if err != nil {
		return Result{}, err
	}
	if input.Policy.ProjectAction != "" {
		if !s.AuthorizeActor(ctx, input.UserID, input.SessionID, projectID, input.Policy) {
			return Result{}, ErrForbidden
		}
	}
	const limit = 20
	switch input.OperationID {
	case "getDashboard":
		var projects []map[string]any
		query := s.db.WithContext(ctx).Table("projects").Select("projects.id, projects.name, projects.identifier")
		if !s.platformAdmin(ctx, input.UserID) {
			query = query.Joins("join project_members on project_members.project_id = projects.id").
				Where("project_members.user_id = ?", input.UserID)
		}
		err := query.Order("projects.updated_at desc").Limit(limit).Scan(&projects).Error
		return databaseResult(Result{Value: map[string]any{"projects": projects}, Truncated: len(projects) == limit}, err)
	case "listProjects":
		var projects []map[string]any
		query := s.db.WithContext(ctx).Table("projects").
			Select("projects.id, projects.name, projects.identifier, projects.description, project_members.role, projects.created_at, projects.updated_at").
			Joins("left join project_members on project_members.project_id = projects.id and project_members.user_id = ?", input.UserID)
		if !s.platformAdmin(ctx, input.UserID) {
			query = query.Where("project_members.user_id = ?", input.UserID)
		}
		err := query.Order("projects.updated_at desc").Limit(limit).Scan(&projects).Error
		return databaseResult(Result{Value: map[string]any{"items": projects}, Truncated: len(projects) == limit}, err)
	case "createProject":
		webConsoleEnabled, _ := input.Arguments["webConsoleEnabled"].(bool)
		webConsoleConfigured := input.Arguments["webConsoleEnabled"] != nil
		var webConsole *bool
		if webConsoleConfigured {
			webConsole = &webConsoleEnabled
		}
		project, err := projectservice.NewService(s.db).Create(ctx, input.UserID, projectservice.CreateInput{
			Identifier:          stringArgument(input.Arguments, "identifier"),
			Name:                stringArgument(input.Arguments, "name"),
			Description:         stringArgument(input.Arguments, "description"),
			NamespaceStrategy:   stringArgument(input.Arguments, "namespaceStrategy"),
			MaxConcurrentBuilds: intArgument(input.Arguments, "maxConcurrentBuilds"),
			WebConsoleEnabled:   webConsole,
		})
		switch {
		case errors.Is(err, projectservice.ErrIdentifierInvalid), errors.Is(err, projectservice.ErrInputInvalid):
			return Result{}, ErrInvalidInput
		case errors.Is(err, projectservice.ErrIdentifierExists):
			return Result{}, ErrConflict
		default:
			return databaseResult(Result{Value: project}, err)
		}
	case "getProject":
		var item map[string]any
		err := s.db.WithContext(ctx).Table("projects").
			Select("id, name, identifier, description, created_at, updated_at").
			Where("id = ?", projectID).Take(&item).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Result{}, ErrNotFound
		}
		return databaseResult(Result{Value: item}, err)
	case "listApplications":
		return s.scanProjectRowsWhere(ctx, "applications", projectID, applicationListColumns, "deleted_at is null", limit)
	case "listBuildRuns":
		return s.scanProjectRows(ctx, "build_runs", projectID, "id, application_id, status, created_at, updated_at", limit)
	case "listReleases":
		return s.scanProjectRows(ctx, "releases", projectID, "id, application_id, status, created_at, updated_at", limit)
	case "listPlatformEvents":
		return s.scanProjectRows(ctx, "platform_events", projectID, "id, type, category, severity, status, resource_type, resource_id, occurred_at", limit)
	case "listGatewayRoutes":
		return s.scanProjectRowsWhere(ctx, "gateway_routes", projectID,
			"id, application_id, deployment_target_id, host, path, tls_mode, dns_status, certificate_status, status, enabled, updated_at",
			"deleted_at is null", limit)
	case "listGatewayCertificates":
		return s.scanProjectRowsWhere(ctx, "gateway_routes", projectID,
			"id, application_id, host, tls_mode, certificate_status, certificate_message, certificate_not_after, certificate_issuer_kind, certificate_issuer_name, updated_at",
			"deleted_at is null and tls_mode <> 'http-only'", limit)
	case "listProjectHookRuns":
		return s.scanProjectRows(ctx, "hook_runs", projectID,
			"id, hook_config_id, build_run_id, release_id, application_id, name, phase, status, exit_code, message, started_at, finished_at, created_at", limit)
	case "listNotificationDeliveries":
		return s.scanProjectRows(ctx, "notification_deliveries", projectID,
			"id, event_id, event_type, severity, channel_id, adapter_kind, status, attempt_count, duration_millis, error_message, queued_at, started_at, finished_at", limit)
	case "listRuntimeEvents":
		return s.scanProjectRowsWhere(ctx, "platform_events", projectID,
			"id, type, category, severity, status, application_id, deployment_target_id, resource_type, resource_id, summary_key, message, correlation_id, occurred_at",
			"(category = 'runtime' or resource_type in ('pod', 'deployment', 'statefulset', 'job', 'runtime_cluster'))", limit)
	case "listRuntimeClusters":
		var rows []map[string]any
		projectResources := s.db.WithContext(ctx).Table("scoped_resource_project_bindings").Select("resource_id").
			Where("resource_type = ? and project_id = ?", "runtime_cluster", projectID)
		err := s.db.WithContext(ctx).Table("runtime_clusters").Select("id, name, type, scope, status, created_at, updated_at").
			Where("scope = 'global' or (scope = 'user' and owner_ref = ?) or (scope = 'project' and id in (?))", input.UserID, projectResources).
			Order("created_at desc").Limit(limit).Scan(&rows).Error
		return databaseResult(Result{Value: map[string]any{"items": rows}, Truncated: len(rows) == limit}, err)
	default:
		return Result{}, ErrForbidden
	}
}

func targetProjectID(policy Policy, arguments map[string]any) (string, error) {
	projectID := strings.TrimSpace(stringArgument(arguments, "projectId"))
	if policy.ProjectAction != "" && projectID == "" {
		return "", ErrInvalidInput
	}
	if policy.ProjectAction == "" && projectID != "" {
		// Platform-scoped tools must never inherit a project from page context.
		return "", ErrInvalidInput
	}
	return projectID, nil
}

func (s *Service) platformAdmin(ctx context.Context, userID string) bool {
	var count int64
	return s.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ? and role = ? and disabled = ?", userID, "platform_admin", false).
		Count(&count).Error == nil && count > 0
}

func stringArgument(arguments map[string]any, key string) string {
	value, _ := arguments[key].(string)
	return value
}

func intArgument(arguments map[string]any, key string) int {
	switch value := arguments[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 0
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
	return databaseResult(Result{Value: map[string]any{"items": rows}, Truncated: len(rows) == limit}, err)
}

func databaseResult(result Result, err error) (Result, error) {
	if err == nil {
		return result, nil
	}
	return Result{}, fmt.Errorf("%w: %v", ErrStorage, err)
}
