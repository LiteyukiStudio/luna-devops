package aitool

import (
	"errors"
	"math"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

// ResolveProject resolves a project by primary key ID (`prj_xxx`) or unique `identifier`.
func ResolveProject(db *gorm.DB, ref string) (model.Project, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.Project{}, BadRequest("project.required", "project is required")
	}
	var project model.Project
	if err := db.Where("(id = ? OR identifier = ?) AND deleted_at IS NULL", ref, ref).
		Order(gorm.Expr("CASE WHEN id = ? THEN 0 ELSE 1 END", ref)).
		First(&project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Project{}, NotFound("project.not_found", "project was not found")
		}
		return model.Project{}, NewError(500, "database.query_failed", err.Error())
	}
	return project, nil
}

// ResolveApplication resolves an application within projectID by primary key ID (`app_xxx`),
// unique `identifier`, or unique display `name` (returning 409 ai.resource_ambiguous if name collides).
func ResolveApplication(db *gorm.DB, projectID, ref string) (model.Application, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.Application{}, BadRequest("application.required", "application is required")
	}
	var app model.Application
	if err := db.Where("project_id = ? AND (id = ? OR identifier = ?) AND deleted_at IS NULL", projectID, ref, ref).
		Order(gorm.Expr("CASE WHEN id = ? THEN 0 ELSE 1 END", ref)).
		First(&app).Error; err == nil {
		return app, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Application{}, NewError(500, "database.query_failed", err.Error())
	}

	var byName []model.Application
	if err := db.Where("project_id = ? AND name = ? AND deleted_at IS NULL", projectID, ref).
		Order("created_at asc").
		Find(&byName).Error; err != nil {
		return model.Application{}, NewError(500, "database.query_failed", err.Error())
	}
	if len(byName) == 1 {
		return byName[0], nil
	}
	if len(byName) > 1 {
		candidates := make([]AmbiguousCandidate, len(byName))
		for i, item := range byName {
			candidates[i] = AmbiguousCandidate{ID: item.ID, Name: item.Name, Identifier: item.Identifier}
		}
		return model.Application{}, AmbiguousResource("application", ref, candidates)
	}
	return model.Application{}, NotFound("application.not_found", "application was not found")
}

// ResolveDeploymentTarget resolves a deployment target within (projectID, applicationID)
// by primary key ID (`dplt_xxx`), unique `stage` (`dev|test|staging|prod`), or unique `name`.
func ResolveDeploymentTarget(db *gorm.DB, projectID, applicationID, ref string) (model.DeploymentTarget, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return model.DeploymentTarget{}, BadRequest("deployment_target.required", "target is required")
	}
	var byIDOrStage model.DeploymentTarget
	if err := db.Where("project_id = ? AND application_id = ? AND (id = ? OR stage = ?) AND deleted_at IS NULL",
		projectID, applicationID, ref, ref).
		Order(gorm.Expr("CASE WHEN id = ? THEN 0 ELSE 1 END", ref)).
		First(&byIDOrStage).Error; err == nil {
		return byIDOrStage, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.DeploymentTarget{}, NewError(500, "database.query_failed", err.Error())
	}

	var byName []model.DeploymentTarget
	if err := db.Where("project_id = ? AND application_id = ? AND name = ? AND deleted_at IS NULL",
		projectID, applicationID, ref).
		Order("created_at asc").
		Find(&byName).Error; err != nil {
		return model.DeploymentTarget{}, NewError(500, "database.query_failed", err.Error())
	}
	if len(byName) == 1 {
		return byName[0], nil
	}
	if len(byName) > 1 {
		candidates := make([]AmbiguousCandidate, len(byName))
		for i, item := range byName {
			candidates[i] = AmbiguousCandidate{ID: item.ID, Name: item.Name, Stage: item.Stage}
		}
		return model.DeploymentTarget{}, AmbiguousResource("deployment_target", ref, candidates)
	}
	return model.DeploymentTarget{}, NotFound("deployment_target.not_found", "deployment target was not found")
}

// ResolveUniqueByIDOrName resolves a single item from an already-scoped slice by exact ID
// first, then exact Name (case-sensitive first, case-insensitive fallback), returning
// 409 ai.resource_ambiguous if multiple items share the same name.
func ResolveUniqueByIDOrName[T any](
	items []T,
	resourceKind string,
	notFoundCode string,
	ref string,
	getID func(T) string,
	getName func(T) string,
) (T, error) {
	var zero T
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return zero, BadRequest(resourceKind+".required", resourceKind+" is required")
	}
	for _, item := range items {
		if getID(item) == ref {
			return item, nil
		}
	}
	var exactMatches []T
	for _, item := range items {
		if getName(item) == ref {
			exactMatches = append(exactMatches, item)
		}
	}
	if len(exactMatches) == 1 {
		return exactMatches[0], nil
	}
	if len(exactMatches) > 1 {
		candidates := make([]AmbiguousCandidate, len(exactMatches))
		for i, item := range exactMatches {
			candidates[i] = AmbiguousCandidate{ID: getID(item), Name: getName(item)}
		}
		return zero, AmbiguousResource(resourceKind, ref, candidates)
	}
	var ciMatches []T
	for _, item := range items {
		if strings.EqualFold(getName(item), ref) {
			ciMatches = append(ciMatches, item)
		}
	}
	if len(ciMatches) == 1 {
		return ciMatches[0], nil
	}
	if len(ciMatches) > 1 {
		candidates := make([]AmbiguousCandidate, len(ciMatches))
		for i, item := range ciMatches {
			candidates[i] = AmbiguousCandidate{ID: getID(item), Name: getName(item)}
		}
		return zero, AmbiguousResource(resourceKind, ref, candidates)
	}
	return zero, NotFound(notFoundCode, resourceKind+" was not found")
}

// NormalizePagination bounds page (>=1) and pageSize (defaultSize..maxSize).
func NormalizePagination(page, pageSize *int, defaultSize, maxSize int) (int, int) {
	if defaultSize <= 0 {
		defaultSize = 20
	}
	if maxSize <= 0 {
		maxSize = 50
	}
	p := 1
	if page != nil && *page > 1 {
		p = *page
	}
	ps := defaultSize
	if pageSize != nil && *pageSize > 0 {
		ps = *pageSize
	}
	if ps > maxSize {
		ps = maxSize
	}
	return p, ps
}

// PageEnvelope builds the standard AGENTS.md §7 paginated list response.
func PageEnvelope[T any](items []T, page, pageSize int, sortBy, sortOrder string, total int64) map[string]any {
	if items == nil {
		items = []T{}
	}
	totalPages := 0
	if total > 0 && pageSize > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(pageSize)))
	}
	return map[string]any{
		"items":      items,
		"page":       page,
		"pageSize":   pageSize,
		"sortBy":     sortBy,
		"sortOrder":  sortOrder,
		"total":      total,
		"totalPages": totalPages,
	}
}
