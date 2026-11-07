package platformevent

import (
	"context"
	"fmt"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

// resourceNameRow is the current display name of one referenced resource and
// the project that owns it. ProjectID is compared with the event's project so
// a name is never resolved across project boundaries.
type resourceNameRow struct {
	ID        string
	ProjectID string
	Name      string
}

type resourceNameResolver func(db *gorm.DB, ids []string) ([]resourceNameRow, error)

// resourceNameResolvers covers every resource type written by event producers
// (notification.Service.Emit, the Git webhook rejection and billing owner
// transfer). Unknown types are left unresolved and fall back to the raw ID.
var resourceNameResolvers = map[string]resourceNameResolver{
	"project":            resolveProjectNames,
	"build":              resolveBuildRunNames,
	"release":            resolveReleaseNames,
	"hook":               resolveHookRunNames,
	"gateway":            resolveGatewayRouteNames,
	"certificate":        resolveGatewayRouteNames,
	"service_binding":    resolveServiceBindingNames,
	"repository_binding": resolveRepositoryBindingNames,
}

// ResourceNames resolves the current display name of each event's primary
// resource, keyed by event ID. Lookups are batched per resource type, limited
// to the event's own project, and omit deleted or missing resources so callers
// fall back to the stable resource ID instead of a fabricated name.
func (s Service) ResourceNames(ctx context.Context, events []model.PlatformEvent) (map[string]string, error) {
	names := map[string]string{}
	if s.DB == nil || len(events) == 0 {
		return names, nil
	}
	idsByType := map[string][]string{}
	seen := map[string]struct{}{}
	for _, event := range events {
		if _, ok := resourceNameResolvers[event.ResourceType]; !ok || event.ResourceID == "" || event.ProjectID == "" {
			continue
		}
		key := event.ResourceType + "\x00" + event.ResourceID
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		idsByType[event.ResourceType] = append(idsByType[event.ResourceType], event.ResourceID)
	}

	db := s.DB.WithContext(ctx)
	resolved := map[string]resourceNameRow{}
	for resourceType, ids := range idsByType {
		rows, err := resourceNameResolvers[resourceType](db, ids)
		if err != nil {
			return nil, fmt.Errorf("resolve %s event resource names: %w", resourceType, err)
		}
		for _, row := range rows {
			if row.Name != "" {
				resolved[resourceType+"\x00"+row.ID] = row
			}
		}
	}
	for _, event := range events {
		row, ok := resolved[event.ResourceType+"\x00"+event.ResourceID]
		if ok && row.ProjectID == event.ProjectID {
			names[event.ID] = row.Name
		}
	}
	return names, nil
}

func resolveProjectNames(db *gorm.DB, ids []string) ([]resourceNameRow, error) {
	var projects []model.Project
	if err := db.Select("id", "name").Where("id in ?", ids).Find(&projects).Error; err != nil {
		return nil, err
	}
	rows := make([]resourceNameRow, 0, len(projects))
	for _, project := range projects {
		rows = append(rows, resourceNameRow{ID: project.ID, ProjectID: project.ID, Name: strings.TrimSpace(project.Name)})
	}
	return rows, nil
}

func resolveBuildRunNames(db *gorm.DB, ids []string) ([]resourceNameRow, error) {
	var runs []model.BuildRun
	if err := db.Select("id", "project_id", "source_branch", "source_tag", "source_commit").Where("id in ?", ids).Find(&runs).Error; err != nil {
		return nil, err
	}
	rows := make([]resourceNameRow, 0, len(runs))
	for _, run := range runs {
		rows = append(rows, resourceNameRow{ID: run.ID, ProjectID: run.ProjectID, Name: buildRunDisplayName(run)})
	}
	return rows, nil
}

// buildRunDisplayName names a build run by its source revision, e.g.
// "main@1a2b3c4"; a run without source metadata has no display name.
func buildRunDisplayName(run model.BuildRun) string {
	ref := strings.TrimSpace(run.SourceTag)
	if ref == "" {
		ref = strings.TrimSpace(run.SourceBranch)
	}
	commit := strings.TrimSpace(run.SourceCommit)
	if len(commit) > 7 {
		commit = commit[:7]
	}
	switch {
	case ref != "" && commit != "":
		return ref + "@" + commit
	case ref != "":
		return ref
	default:
		return commit
	}
}

func resolveReleaseNames(db *gorm.DB, ids []string) ([]resourceNameRow, error) {
	var releases []model.Release
	if err := db.Select("id", "project_id", "revision").Where("id in ?", ids).Find(&releases).Error; err != nil {
		return nil, err
	}
	rows := make([]resourceNameRow, 0, len(releases))
	for _, release := range releases {
		rows = append(rows, resourceNameRow{ID: release.ID, ProjectID: release.ProjectID, Name: fmt.Sprintf("#%d", release.Revision)})
	}
	return rows, nil
}

func resolveHookRunNames(db *gorm.DB, ids []string) ([]resourceNameRow, error) {
	var runs []model.HookRun
	if err := db.Select("id", "project_id", "name").Where("id in ?", ids).Find(&runs).Error; err != nil {
		return nil, err
	}
	rows := make([]resourceNameRow, 0, len(runs))
	for _, run := range runs {
		rows = append(rows, resourceNameRow{ID: run.ID, ProjectID: run.ProjectID, Name: strings.TrimSpace(run.Name)})
	}
	return rows, nil
}

func resolveGatewayRouteNames(db *gorm.DB, ids []string) ([]resourceNameRow, error) {
	var routes []model.GatewayRoute
	if err := db.Select("id", "project_id", "host", "path").Where("id in ?", ids).Find(&routes).Error; err != nil {
		return nil, err
	}
	rows := make([]resourceNameRow, 0, len(routes))
	for _, route := range routes {
		rows = append(rows, resourceNameRow{ID: route.ID, ProjectID: route.ProjectID, Name: gatewayRouteDisplayName(route)})
	}
	return rows, nil
}

// gatewayRouteDisplayName names a route by host plus a non-root path, e.g.
// "blog.example.com/api".
func gatewayRouteDisplayName(route model.GatewayRoute) string {
	host := strings.TrimSpace(route.Host)
	if host == "" {
		return ""
	}
	path := strings.TrimSpace(route.Path)
	if path == "" || path == "/" {
		return host
	}
	return host + "/" + strings.TrimLeft(path, "/")
}

// resolveServiceBindingNames names a binding by the service it references,
// e.g. "blog-api:http". The target application must belong to the binding's
// project and still exist.
func resolveServiceBindingNames(db *gorm.DB, ids []string) ([]resourceNameRow, error) {
	var bindings []model.ServiceBinding
	if err := db.Select("id", "project_id", "target_application_id", "target_port_name").Where("id in ?", ids).Find(&bindings).Error; err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return nil, nil
	}
	applicationIDs := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		applicationIDs = append(applicationIDs, binding.TargetApplicationID)
	}
	var applications []model.Application
	if err := db.Select("id", "project_id", "name").Where("id in ?", applicationIDs).Find(&applications).Error; err != nil {
		return nil, err
	}
	applicationsByID := make(map[string]model.Application, len(applications))
	for _, application := range applications {
		applicationsByID[application.ID] = application
	}
	rows := make([]resourceNameRow, 0, len(bindings))
	for _, binding := range bindings {
		application, ok := applicationsByID[binding.TargetApplicationID]
		applicationName := strings.TrimSpace(application.Name)
		if !ok || application.ProjectID != binding.ProjectID || applicationName == "" {
			continue
		}
		name := applicationName
		if port := strings.TrimSpace(binding.TargetPortName); port != "" {
			name += ":" + port
		}
		rows = append(rows, resourceNameRow{ID: binding.ID, ProjectID: binding.ProjectID, Name: name})
	}
	return rows, nil
}

func resolveRepositoryBindingNames(db *gorm.DB, ids []string) ([]resourceNameRow, error) {
	var bindings []model.RepositoryBinding
	if err := db.Select("id", "project_id", "owner", "repo").Where("id in ?", ids).Find(&bindings).Error; err != nil {
		return nil, err
	}
	rows := make([]resourceNameRow, 0, len(bindings))
	for _, binding := range bindings {
		owner, repo := strings.TrimSpace(binding.Owner), strings.TrimSpace(binding.Repo)
		if repo == "" {
			continue
		}
		name := repo
		if owner != "" {
			name = owner + "/" + repo
		}
		rows = append(rows, resourceNameRow{ID: binding.ID, ProjectID: binding.ProjectID, Name: name})
	}
	return rows, nil
}
