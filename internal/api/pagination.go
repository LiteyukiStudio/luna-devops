package api

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"strconv"
	"strings"
)

type paginationParams struct {
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

func (p paginationParams) Offset() int {
	return (p.Page - 1) * p.PageSize
}

func paginationFromQuery(ctx *gin.Context) paginationParams {
	page := parsePositiveInt(ctx.Query("page"), 1)
	pageSize := parsePositiveInt(ctx.Query("pageSize"), 20)
	if pageSize > 100 {
		pageSize = 100
	}
	sortOrder := strings.ToLower(ctx.Query("sortOrder"))
	if sortOrder != "asc" {
		sortOrder = "desc"
	}
	return paginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    ctx.Query("sortBy"),
		SortOrder: sortOrder,
	}
}

func paginatedResponse[T any](items []T, total int64, pagination paginationParams) gin.H {
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(pagination.PageSize) - 1) / int64(pagination.PageSize))
	}
	return gin.H{
		"items":      items,
		"page":       pagination.Page,
		"pageSize":   pagination.PageSize,
		"sortBy":     pagination.SortBy,
		"sortOrder":  pagination.SortOrder,
		"total":      total,
		"totalPages": totalPages,
	}
}

func paginateSlice[T any](items []T, pagination paginationParams) []T {
	start := pagination.Offset()
	if start >= len(items) {
		return []T{}
	}
	end := start + pagination.PageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func orderByClause(pagination paginationParams, allowedFields map[string]string, defaultColumn string) string {
	column := allowedFields[pagination.SortBy]
	if column == "" {
		column = defaultColumn
	}

	order := pagination.SortOrder
	if order != "asc" {
		order = "desc"
	}
	return column + " " + order
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

// applySearch appends a case-insensitive LIKE filter over the given columns
// when the request carries a non-empty "search" query parameter. When the
// keyword is empty the query is returned unchanged, so existing callers keep
// their current behaviour. LIKE wildcards in the keyword are escaped.
func applySearch(ctx *gin.Context, query *gorm.DB, columns ...string) *gorm.DB {
	keyword := strings.TrimSpace(ctx.Query("search"))
	if keyword == "" || len(columns) == 0 {
		return query
	}

	escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(keyword)
	pattern := "%" + escaped + "%"

	conditions := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns))
	for _, column := range columns {
		conditions = append(conditions, "LOWER("+column+") LIKE LOWER(?) ESCAPE '\\'")
		args = append(args, pattern)
	}
	return query.Where(strings.Join(conditions, " OR "), args...)
}
