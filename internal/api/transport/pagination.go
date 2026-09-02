package transport

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"strconv"
	"strings"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
	DefaultPageSize = defaultPageSize
	MaxPageSize     = maxPageSize
)

type PaginationParams struct {
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

type PaginatedResponseBody[T any] struct {
	Items      []T    `json:"items"`
	Page       int    `json:"page"`
	PageSize   int    `json:"pageSize"`
	SortBy     string `json:"sortBy"`
	SortOrder  string `json:"sortOrder"`
	Total      int64  `json:"total"`
	TotalPages int    `json:"totalPages"`
}

func (p PaginationParams) Offset() int {
	return (p.Page - 1) * p.PageSize
}

func PaginationFromQuery(ctx *gin.Context) PaginationParams {
	page := ParsePositiveInt(ctx.Query("page"), 1)
	pageSize := ParsePositiveInt(ctx.Query("pageSize"), defaultPageSize)
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	sortOrder := strings.ToLower(ctx.Query("sortOrder"))
	if sortOrder != "asc" {
		sortOrder = "desc"
	}
	return PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    ctx.Query("sortBy"),
		SortOrder: sortOrder,
	}
}

func PaginationFromQueryWithSort(ctx *gin.Context, allowedFields map[string]string, defaultField string) PaginationParams {
	pagination := PaginationFromQuery(ctx)
	if allowedFields[pagination.SortBy] == "" {
		pagination.SortBy = defaultField
	}
	return pagination
}

func PaginatedResponse[T any](items []T, total int64, pagination PaginationParams) PaginatedResponseBody[T] {
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(pagination.PageSize) - 1) / int64(pagination.PageSize))
	}
	return PaginatedResponseBody[T]{
		Items:      items,
		Page:       pagination.Page,
		PageSize:   pagination.PageSize,
		SortBy:     pagination.SortBy,
		SortOrder:  pagination.SortOrder,
		Total:      total,
		TotalPages: totalPages,
	}
}

func PaginateSlice[T any](items []T, pagination PaginationParams) []T {
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

func OrderByClause(pagination PaginationParams, allowedFields map[string]string, defaultColumn string) string {
	column := allowedFields[pagination.SortBy]
	if column == "" {
		// defaultColumn 约定为裸列名；历史调用方误传入方向后缀时在此归一化，
		// 避免拼出 "col desc desc" 之类的非法 SQL。
		column = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(defaultColumn, " desc"), " asc"))
	}

	order := pagination.SortOrder
	if order != "asc" {
		order = "desc"
	}
	return column + " " + order
}

func ParsePositiveInt(value string, fallback int) int {
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
func ApplySearch(ctx *gin.Context, query *gorm.DB, columns ...string) *gorm.DB {
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
