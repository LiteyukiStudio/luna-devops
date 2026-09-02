package api

import (
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const terminalDisconnectedErrorCode = transportapi.TerminalDisconnectedErrorCode
const defaultPageSize = transportapi.DefaultPageSize
const maxPageSize = transportapi.MaxPageSize

type paginationParams = transportapi.PaginationParams
type paginatedResponseBody[T any] = transportapi.PaginatedResponseBody[T]

func requestIDMiddleware() gin.HandlerFunc { return transportapi.RequestIDMiddleware() }
func runtimeModeMiddleware(mode string) gin.HandlerFunc {
	return transportapi.RuntimeModeMiddleware(mode)
}
func setRuntimeMode(ctx *gin.Context, mode string) { transportapi.SetRuntimeMode(ctx, mode) }
func isDevelopmentRequest(ctx *gin.Context) bool   { return transportapi.IsDevelopmentRequest(ctx) }
func requestID(ctx *gin.Context) string            { return transportapi.RequestID(ctx) }
func bindJSON(ctx *gin.Context, value any) bool    { return transportapi.BindJSON(ctx, value) }
func writeError(ctx *gin.Context, status int, message string) {
	transportapi.WriteError(ctx, status, message)
}
func internalErrorCode(ctx *gin.Context) string { return transportapi.InternalErrorCode(ctx) }
func writeErrorKey(ctx *gin.Context, status int, message, key string) {
	transportapi.WriteErrorKey(ctx, status, message, key)
}
func writeErrorKeyWithDetails(ctx *gin.Context, status int, message, key string, details gin.H) {
	transportapi.WriteErrorKeyWithDetails(ctx, status, message, key, details)
}
func writeScopeInsufficientError(ctx *gin.Context, requiredScope string) {
	transportapi.WriteScopeInsufficientError(ctx, requiredScope)
}
func writeScopeContractUnavailableError(ctx *gin.Context, detail string) {
	transportapi.WriteScopeContractUnavailableError(ctx, detail)
}
func writeErrorCode(ctx *gin.Context, status int, code, detail string) {
	transportapi.WriteErrorCode(ctx, status, code, detail)
}
func writeArgumentErrorCode(ctx *gin.Context, status int, code, detail, path string, allowedValues []string, retryable bool) {
	transportapi.WriteArgumentErrorCode(ctx, status, code, detail, path, allowedValues, retryable)
}
func writeLocalizedErrorCode(ctx *gin.Context, status int, code, detail, publicMessageKey string) {
	transportapi.WriteLocalizedErrorCode(ctx, status, code, detail, publicMessageKey)
}
func errorEnvelope(ctx *gin.Context, status int, code string) gin.H {
	return transportapi.ErrorEnvelope(ctx, status, code)
}
func traceID(ctx *gin.Context) string          { return transportapi.TraceID(ctx) }
func publicErrorMessageKey(status int) string  { return transportapi.PublicErrorMessageKey(status) }
func errorResponseMiddleware() gin.HandlerFunc { return transportapi.ErrorResponseMiddleware() }
func recoveryMiddleware() gin.HandlerFunc      { return transportapi.RecoveryMiddleware() }
func terminalDisconnectedMessage(ctx *gin.Context, detail string) []byte {
	return transportapi.TerminalDisconnectedMessage(ctx, detail)
}
func defaultErrorCode(status int) string         { return transportapi.DefaultErrorCode(status) }
func messageFor(language, key string) string     { return transportapi.MessageFor(language, key) }
func normalizeLanguage(language string) string   { return transportapi.NormalizeLanguage(language) }
func requestLanguage(ctx *gin.Context) string    { return transportapi.RequestLanguage(ctx) }
func fallback(value, defaultValue string) string { return transportapi.Fallback(value, defaultValue) }
func fallbackInt(value, defaultValue int) int    { return transportapi.FallbackInt(value, defaultValue) }
func randomHex(length int) string                { return transportapi.RandomHex(length) }
func hashToken(token string) string              { return transportapi.HashToken(token) }
func normalizeStringList(values []string) []string {
	return transportapi.NormalizeStringList(values)
}

func paginationFromQuery(ctx *gin.Context) paginationParams {
	return transportapi.PaginationFromQuery(ctx)
}
func paginationFromQueryWithSort(ctx *gin.Context, allowedFields map[string]string, defaultField string) paginationParams {
	return transportapi.PaginationFromQueryWithSort(ctx, allowedFields, defaultField)
}
func paginatedResponse[T any](items []T, total int64, pagination paginationParams) paginatedResponseBody[T] {
	return transportapi.PaginatedResponse(items, total, pagination)
}
func paginateSlice[T any](items []T, pagination paginationParams) []T {
	return transportapi.PaginateSlice(items, pagination)
}
func orderByClause(pagination paginationParams, allowedFields map[string]string, defaultColumn string) string {
	return transportapi.OrderByClause(pagination, allowedFields, defaultColumn)
}
func parsePositiveInt(value string, fallbackValue int) int {
	return transportapi.ParsePositiveInt(value, fallbackValue)
}
func applySearch(ctx *gin.Context, query *gorm.DB, columns ...string) *gorm.DB {
	return transportapi.ApplySearch(ctx, query, columns...)
}
func markLiveObservationResponse(ctx *gin.Context) {
	transportapi.MarkLiveObservationResponse(ctx)
}
