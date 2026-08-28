package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/gin-gonic/gin"
)

func TestResolveListVisibilityFromQuery(t *testing.T) {
	tests := []struct {
		name           string
		role           string
		query          string
		want           projectservice.ListVisibility
		wantOK         bool
		wantHTTPStatus int
	}{
		{name: "user default related", role: authz.PlatformRoleUser, want: projectservice.ListVisibilityRelated, wantOK: true, wantHTTPStatus: http.StatusOK},
		{name: "admin default related", role: authz.PlatformRoleAdmin, want: projectservice.ListVisibilityRelated, wantOK: true, wantHTTPStatus: http.StatusOK},
		{name: "admin explicit all", role: authz.PlatformRoleAdmin, query: "?visibility=all", want: projectservice.ListVisibilityAll, wantOK: true, wantHTTPStatus: http.StatusOK},
		{name: "user explicit all forbidden", role: authz.PlatformRoleUser, query: "?visibility=all", wantOK: false, wantHTTPStatus: http.StatusForbidden},
		{name: "invalid value", role: authz.PlatformRoleAdmin, query: "?visibility=mine", wantOK: false, wantHTTPStatus: http.StatusBadRequest},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/projects"+test.query, nil)

			visibility, ok := resolveListVisibility(ctx, model.User{ID: "usr_visibility", Role: test.role})
			if ok != test.wantOK || visibility != test.want {
				t.Fatalf("visibility=%q ok=%t, want visibility=%q ok=%t", visibility, ok, test.want, test.wantOK)
			}
			if recorder.Code != test.wantHTTPStatus {
				t.Fatalf("status=%d, want %d; body=%s", recorder.Code, test.wantHTTPStatus, recorder.Body.String())
			}
		})
	}
}
