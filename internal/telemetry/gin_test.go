package telemetry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestQueryTraceContextMiddlewareMovesPrivateParametersToHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(QueryTraceContextMiddleware())
	router.GET("/events", func(ctx *gin.Context) {
		if got := ctx.GetHeader("traceparent"); got != "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01" {
			t.Errorf("traceparent = %q", got)
		}
		if got := ctx.Query("after"); got != "42" {
			t.Errorf("after = %q", got)
		}
		if ctx.Request.URL.Query().Has("_otel_traceparent") {
			t.Error("private trace parameter reached handler")
		}
		ctx.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/events?after=42&_otel_traceparent=00-0123456789abcdef0123456789abcdef-0123456789abcdef-01", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
}
