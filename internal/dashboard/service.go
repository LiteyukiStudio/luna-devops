package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

const (
	projectShortcutLimit = 16
	activityLimit        = 20
	eventScanLimit       = 300
	attentionLimit       = 8
	attentionWindow      = 7 * 24 * time.Hour
)

type Scope struct {
	UserID            string
	AllProjects       bool
	VisibleProjectIDs []string
}

type Overview struct {
	GeneratedAt time.Time         `json:"generatedAt"`
	Summary     Summary           `json:"summary"`
	Projects    []ProjectShortcut `json:"projects"`
	Attention   []AttentionItem   `json:"attention"`
	Activities  []Activity        `json:"activities"`
	Readiness   Readiness         `json:"readiness"`
}

type Summary struct {
	Projects        int64 `json:"projects"`
	Applications    int64 `json:"applications"`
	ActiveBuilds    int64 `json:"activeBuilds"`
	ActiveReleases  int64 `json:"activeReleases"`
	AttentionItems  int   `json:"attentionItems"`
	HealthyClusters int   `json:"healthyClusters"`
	TotalClusters   int   `json:"totalClusters"`
}

type EntityRef struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
}

type ProjectShortcut struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Identifier       string    `json:"identifier"`
	Description      string    `json:"description"`
	Pinned           bool      `json:"pinned"`
	ApplicationCount int64     `json:"applicationCount"`
	LatestActivity   *Activity `json:"latestActivity"`
}

type AttentionItem struct {
	Key         string   `json:"key"`
	Category    string   `json:"category"`
	Severity    string   `json:"severity"`
	Occurrences int      `json:"occurrences"`
	Latest      Activity `json:"latest"`
}

type Activity struct {
	ID               string            `json:"id"`
	Type             string            `json:"type"`
	Category         string            `json:"category"`
	Severity         string            `json:"severity"`
	Status           string            `json:"status"`
	Message          string            `json:"message"`
	Project          *EntityRef        `json:"project,omitempty"`
	Application      *EntityRef        `json:"application,omitempty"`
	DeploymentTarget *EntityRef        `json:"deploymentTarget,omitempty"`
	ResourceType     string            `json:"resourceType"`
	ResourceID       string            `json:"resourceId"`
	Links            map[string]string `json:"links"`
	OccurredAt       time.Time         `json:"occurredAt"`
}

type Readiness struct {
	Clusters   ReadinessItem `json:"clusters"`
	Registries ReadinessItem `json:"registries"`
}

type ReadinessItem struct {
	Status          string    `json:"status"`
	Available       int       `json:"available"`
	Total           int       `json:"total"`
	ObservationCode string    `json:"observationCode,omitempty"`
	ObservedAt      time.Time `json:"observedAt"`
}

type Service struct {
	db  *gorm.DB
	now func() time.Time
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db, now: time.Now}
}

func (s *Service) Overview(ctx context.Context, scope Scope) (Overview, error) {
	result := Overview{
		GeneratedAt: s.now().UTC(),
		Projects:    []ProjectShortcut{},
		Attention:   []AttentionItem{},
		Activities:  []Activity{},
	}
	projectIDs, err := s.visibleProjectIDs(ctx, scope)
	if err != nil {
		return result, err
	}
	result.Summary.Projects = int64(len(projectIDs))

	if len(projectIDs) > 0 {
		if err := s.db.WithContext(ctx).Model(&model.Application{}).Where("project_id in ?", projectIDs).Count(&result.Summary.Applications).Error; err != nil {
			return result, fmt.Errorf("count dashboard applications: %w", err)
		}
		if err := s.db.WithContext(ctx).Model(&model.BuildRun{}).Where("project_id in ? and status in ?", projectIDs, []string{"queued", "running"}).Count(&result.Summary.ActiveBuilds).Error; err != nil {
			return result, fmt.Errorf("count dashboard builds: %w", err)
		}
		if err := s.db.WithContext(ctx).Model(&model.Release{}).Where("project_id in ? and status in ?", projectIDs, []string{"pending", "running"}).Count(&result.Summary.ActiveReleases).Error; err != nil {
			return result, fmt.Errorf("count dashboard releases: %w", err)
		}
	}

	events, err := s.recentEvents(ctx, scope, projectIDs)
	if err != nil {
		return result, err
	}
	references, err := s.loadEntityReferences(ctx, projectIDs, events)
	if err != nil {
		return result, err
	}
	activities := make([]Activity, 0, len(events))
	for _, event := range events {
		activities = append(activities, activityFromEvent(event, references))
	}
	if len(activities) > activityLimit {
		result.Activities = activities[:activityLimit]
	} else {
		result.Activities = activities
	}
	result.Attention = aggregateAttention(events, references, result.GeneratedAt.Add(-attentionWindow))
	result.Summary.AttentionItems = len(result.Attention)

	result.Projects, err = s.projectShortcuts(ctx, scope, projectIDs, activities)
	if err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) visibleProjectIDs(ctx context.Context, scope Scope) ([]string, error) {
	query := s.db.WithContext(ctx).Model(&model.Project{}).Select("projects.id").Where("projects.deleted_at is null")
	if !scope.AllProjects {
		if len(scope.VisibleProjectIDs) == 0 {
			return []string{}, nil
		}
		query = query.Where("projects.id in ?", scope.VisibleProjectIDs)
	}
	var ids []string
	if err := query.Order("projects.created_at desc").Scan(&ids).Error; err != nil {
		return nil, fmt.Errorf("list dashboard projects: %w", err)
	}
	return ids, nil
}

type projectRow struct {
	ID          string
	Name        string
	Identifier  string
	Description string
	Pinned      bool
}

type projectCountRow struct {
	ProjectID string
	Count     int64
}

func (s *Service) projectShortcuts(ctx context.Context, scope Scope, projectIDs []string, activities []Activity) ([]ProjectShortcut, error) {
	if len(projectIDs) == 0 {
		return []ProjectShortcut{}, nil
	}
	query := s.db.WithContext(ctx).Table("projects").
		Select("projects.id, projects.name, projects.identifier, projects.description, (project_pins.project_id is not null) as pinned").
		Joins("left join project_members on project_members.project_id = projects.id and project_members.user_id = ?", scope.UserID).
		Joins("left join project_pins on project_pins.project_id = projects.id and project_pins.user_id = ?", scope.UserID).
		Where("projects.deleted_at is null and projects.id in ?", projectIDs).
		Order("(project_pins.project_id is not null) desc, coalesce(project_members.use_count, 0) desc, coalesce(project_members.last_used_at, projects.created_at) desc").
		Limit(projectShortcutLimit)
	var rows []projectRow
	if err := query.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list dashboard project shortcuts: %w", err)
	}
	shortcutIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		shortcutIDs = append(shortcutIDs, row.ID)
	}
	counts := map[string]int64{}
	if len(shortcutIDs) > 0 {
		var countRows []projectCountRow
		if err := s.db.WithContext(ctx).Model(&model.Application{}).
			Select("project_id, count(*) as count").
			Where("project_id in ?", shortcutIDs).
			Group("project_id").Scan(&countRows).Error; err != nil {
			return nil, fmt.Errorf("count dashboard shortcut applications: %w", err)
		}
		for _, row := range countRows {
			counts[row.ProjectID] = row.Count
		}
	}
	latest := map[string]Activity{}
	for _, activity := range activities {
		if activity.Project != nil {
			if _, exists := latest[activity.Project.ID]; !exists {
				latest[activity.Project.ID] = activity
			}
		}
	}
	shortcuts := make([]ProjectShortcut, 0, len(rows))
	for _, row := range rows {
		shortcut := ProjectShortcut{ID: row.ID, Name: row.Name, Identifier: row.Identifier, Description: row.Description, Pinned: row.Pinned, ApplicationCount: counts[row.ID]}
		if activity, exists := latest[row.ID]; exists {
			activityCopy := activity
			shortcut.LatestActivity = &activityCopy
		}
		shortcuts = append(shortcuts, shortcut)
	}
	return shortcuts, nil
}

func (s *Service) recentEvents(ctx context.Context, scope Scope, projectIDs []string) ([]model.PlatformEvent, error) {
	query := s.db.WithContext(ctx).Model(&model.PlatformEvent{})
	if !scope.AllProjects {
		if len(projectIDs) == 0 {
			query = query.Where("actor_id = ?", scope.UserID)
		} else {
			query = query.Where("project_id in ? or actor_id = ?", projectIDs, scope.UserID)
		}
	}
	var events []model.PlatformEvent
	if err := query.Order("occurred_at desc, created_at desc").Limit(eventScanLimit).Find(&events).Error; err != nil {
		return nil, fmt.Errorf("list dashboard events: %w", err)
	}
	return events, nil
}

type entityReferences struct {
	projects map[string]EntityRef
	apps     map[string]EntityRef
	targets  map[string]EntityRef
}

func (s *Service) loadEntityReferences(ctx context.Context, projectIDs []string, events []model.PlatformEvent) (entityReferences, error) {
	references := entityReferences{projects: map[string]EntityRef{}, apps: map[string]EntityRef{}, targets: map[string]EntityRef{}}
	if len(projectIDs) > 0 {
		var projects []model.Project
		if err := s.db.WithContext(ctx).Where("id in ?", projectIDs).Find(&projects).Error; err != nil {
			return references, fmt.Errorf("load dashboard project references: %w", err)
		}
		for _, project := range projects {
			references.projects[project.ID] = EntityRef{ID: project.ID, Name: project.Name, Identifier: project.Identifier}
		}
	}
	applicationIDs := make([]string, 0)
	targetIDs := make([]string, 0)
	seenApps := map[string]struct{}{}
	seenTargets := map[string]struct{}{}
	for _, event := range events {
		if event.ApplicationID != "" {
			if _, exists := seenApps[event.ApplicationID]; !exists {
				seenApps[event.ApplicationID] = struct{}{}
				applicationIDs = append(applicationIDs, event.ApplicationID)
			}
		}
		if event.DeploymentTargetID != "" {
			if _, exists := seenTargets[event.DeploymentTargetID]; !exists {
				seenTargets[event.DeploymentTargetID] = struct{}{}
				targetIDs = append(targetIDs, event.DeploymentTargetID)
			}
		}
	}
	if len(applicationIDs) > 0 {
		var applications []model.Application
		if err := s.db.WithContext(ctx).Where("id in ?", applicationIDs).Find(&applications).Error; err != nil {
			return references, fmt.Errorf("load dashboard application references: %w", err)
		}
		for _, application := range applications {
			references.apps[application.ID] = EntityRef{ID: application.ID, Name: application.Name, Identifier: application.Identifier}
		}
	}
	if len(targetIDs) > 0 {
		var targets []model.DeploymentTarget
		if err := s.db.WithContext(ctx).Where("id in ?", targetIDs).Find(&targets).Error; err != nil {
			return references, fmt.Errorf("load dashboard deployment target references: %w", err)
		}
		for _, target := range targets {
			references.targets[target.ID] = EntityRef{ID: target.ID, Name: target.Name, Identifier: target.Stage}
		}
	}
	return references, nil
}

func activityFromEvent(event model.PlatformEvent, references entityReferences) Activity {
	activity := Activity{
		ID: event.ID, Type: event.Type, Category: event.Category, Severity: event.Severity, Status: event.Status,
		Message: event.Message, ResourceType: event.ResourceType, ResourceID: event.ResourceID,
		Links: map[string]string{}, OccurredAt: event.OccurredAt,
	}
	if ref, exists := references.projects[event.ProjectID]; exists {
		copy := ref
		activity.Project = &copy
	}
	if ref, exists := references.apps[event.ApplicationID]; exists {
		copy := ref
		activity.Application = &copy
	}
	if ref, exists := references.targets[event.DeploymentTargetID]; exists {
		copy := ref
		activity.DeploymentTarget = &copy
	}
	if err := json.Unmarshal([]byte(event.LinksJSON), &activity.Links); err != nil || activity.Links == nil {
		activity.Links = map[string]string{}
	}
	return activity
}

type attentionGroup struct {
	item   AttentionItem
	closed bool
}

func aggregateAttention(events []model.PlatformEvent, references entityReferences, cutoff time.Time) []AttentionItem {
	groups := map[string]*attentionGroup{}
	order := make([]string, 0)
	for _, event := range events {
		if event.OccurredAt.Before(cutoff) {
			continue
		}
		key := attentionGroupKey(event)
		group, exists := groups[key]
		if !exists {
			group = &attentionGroup{}
			groups[key] = group
			order = append(order, key)
			if !eventNeedsAttention(event) {
				group.closed = true
				continue
			}
			group.item = AttentionItem{Key: key, Category: event.Category, Severity: event.Severity, Occurrences: 1, Latest: activityFromEvent(event, references)}
			continue
		}
		if group.closed {
			continue
		}
		if !eventNeedsAttention(event) {
			group.closed = true
			continue
		}
		group.item.Occurrences++
		if severityRank(event.Severity) > severityRank(group.item.Severity) {
			group.item.Severity = event.Severity
		}
	}
	items := make([]AttentionItem, 0, len(groups))
	for _, key := range order {
		group := groups[key]
		if group.item.Occurrences > 0 {
			items = append(items, group.item)
		}
	}
	sort.SliceStable(items, func(left, right int) bool {
		return items[left].Latest.OccurredAt.After(items[right].Latest.OccurredAt)
	})
	if len(items) > attentionLimit {
		return items[:attentionLimit]
	}
	return items
}

func attentionGroupKey(event model.PlatformEvent) string {
	identity := event.DeploymentTargetID
	if identity == "" {
		identity = event.ApplicationID
	}
	if identity == "" {
		identity = event.ResourceID
	}
	if identity == "" {
		identity = event.ProjectID
	}
	if identity == "" {
		identity = event.Type
	}
	return strings.Join([]string{event.Category, event.ProjectID, identity}, ":")
}

func eventNeedsAttention(event model.PlatformEvent) bool {
	return event.Status == "failed" || event.Severity == "error" || event.Severity == "warning"
}

func severityRank(value string) int {
	switch value {
	case "error":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}

func (s *Service) ReadinessResources(ctx context.Context, scope Scope) ([]model.RuntimeCluster, []model.ArtifactRegistry, error) {
	projectIDs, err := s.visibleProjectIDs(ctx, scope)
	if err != nil {
		return nil, nil, err
	}
	clusterQuery := s.scopedResourceQuery(ctx, scope, projectIDs, "runtime_cluster", &model.RuntimeCluster{})
	var clusters []model.RuntimeCluster
	if err := clusterQuery.Find(&clusters).Error; err != nil {
		return nil, nil, fmt.Errorf("list dashboard clusters: %w", err)
	}

	registryQuery := s.scopedResourceQuery(ctx, scope, projectIDs, "artifact_registry", &model.ArtifactRegistry{})
	var registries []model.ArtifactRegistry
	if err := registryQuery.Find(&registries).Error; err != nil {
		return nil, nil, fmt.Errorf("list dashboard registries: %w", err)
	}
	return clusters, registries, nil
}

func (s *Service) scopedResourceQuery(ctx context.Context, scope Scope, projectIDs []string, resourceType string, value any) *gorm.DB {
	query := s.db.WithContext(ctx).Model(value)
	conditions := []string{"scope = 'global'", "(scope = 'user' and owner_ref = ?)"}
	args := []any{scope.UserID}
	if scope.AllProjects {
		conditions = append(conditions, "scope = 'project'")
	} else if len(projectIDs) > 0 {
		bindings := s.db.WithContext(ctx).Model(&model.ScopedResourceProjectBinding{}).Select("resource_id").Where("resource_type = ?", resourceType)
		conditions = append(conditions, "(scope = 'project' and id in (?))")
		args = append(args, bindings.Where("project_id in ?", projectIDs))
	}
	return query.Where(strings.Join(conditions, " or "), args...)
}
