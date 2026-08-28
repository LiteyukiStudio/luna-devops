package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestPlatformAdminRoutesOwnAuthorizationInMiddleware(t *testing.T) {
	db := authIntegrationDB(t)
	user := model.User{
		ID: "usr_platform_admin_middleware", Email: "platform-admin-middleware@example.com",
		Name: "Platform Admin Middleware", Role: authz.PlatformRoleUser, Language: "en-US",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessionToken := "sess_platform_admin_middleware"
	if err := db.Create(&model.UserSession{
		ID: "ses_platform_admin_middleware", UserID: user.ID,
		TokenHash: hashToken(sessionToken), ExpiresAt: time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	router := NewRouter(db)

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodPut, "/api/v1/auth/registration/settings"},
		{http.MethodGet, "/api/v1/mail/settings"},
		{http.MethodPut, "/api/v1/mail/settings"},
		{http.MethodPost, "/api/v1/mail/settings/test"},
		{http.MethodPost, "/api/v1/auth/providers"},
		{http.MethodPut, "/api/v1/auth/providers/ap_missing"},
		{http.MethodGet, "/api/v1/configs/ai/models"},
		{http.MethodPost, "/api/v1/configs/ai/models"},
		{http.MethodPut, "/api/v1/configs/ai/models/aim_missing"},
		{http.MethodDelete, "/api/v1/configs/ai/models/aim_missing"},
		{http.MethodPost, "/api/v1/configs/ai/provider/test"},
		{http.MethodPost, "/api/v1/configs/ai/observability/test"},
		{http.MethodGet, "/api/v1/ai/observability/overview"},
		{http.MethodGet, "/api/v1/ai/observability/conversations"},
		{http.MethodGet, "/api/v1/ai/observability/turns"},
		{http.MethodGet, "/api/v1/ai/observability/tools"},
		{http.MethodGet, "/api/v1/ai/observability/tools/test.operation/calls"},
		{http.MethodGet, "/api/v1/ai/observability/conversations/conv_missing"},
		{http.MethodGet, "/api/v1/ai/observability/traces/trace_missing"},
		{http.MethodPost, "/api/v1/users"},
		{http.MethodPut, "/api/v1/users/usr_missing"},
		{http.MethodPost, "/api/v1/data-retention/cleanup"},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			response := performCookieJSONRequest(router, test.method, test.path, sessionToken, `{}`)
			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
}
