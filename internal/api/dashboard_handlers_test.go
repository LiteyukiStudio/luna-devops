package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func TestGetDashboardValidatesListVisibilityBeforeLoadingData(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		role       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "all requires platform administrator",
			path:       "/api/v1/dashboard?visibility=all",
			role:       authz.PlatformRoleUser,
			wantStatus: http.StatusForbidden,
			wantCode:   "auth.forbidden",
		},
		{
			name:       "unknown visibility is invalid",
			path:       "/api/v1/dashboard?visibility=mine",
			role:       authz.PlatformRoleAdmin,
			wantStatus: http.StatusBadRequest,
			wantCode:   "request.invalid",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, testCase.path, nil)
			ctx.Set(currentUserContextKey, model.User{ID: "usr_dashboard", Role: testCase.role})

			(&Handlers{}).GetDashboard(ctx)

			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, testCase.wantStatus, recorder.Body.String())
			}
			var response struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.Code != testCase.wantCode {
				t.Fatalf("code = %q, want %q", response.Code, testCase.wantCode)
			}
		})
	}
}
