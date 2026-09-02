package identityapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func TestOAuthApplicationResponseNormalizesEmptyRedirectURIs(t *testing.T) {
	response := oauthApplicationToResponse(model.OAuthApplication{})
	if response.RedirectURIs == nil || len(response.RedirectURIs) != 0 {
		t.Fatalf("redirectUris = %#v, want a non-nil empty slice", response.RedirectURIs)
	}

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal OAuth application response: %v", err)
	}
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode OAuth application response: %v", err)
	}
	if got := string(fields["redirectUris"]); got != "[]" {
		t.Fatalf("serialized redirectUris = %s, want []", got)
	}
}

func TestOAuthRevokeRejectsMissingOrBlankToken(t *testing.T) {
	router := gin.New()
	router.POST("/api/v1/oauth/revoke", (&Handlers{}).RevokeOAuthToken)

	for _, test := range []struct {
		name string
		form url.Values
	}{
		{name: "missing", form: url.Values{}},
		{name: "blank", form: url.Values{"token": {"  "}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/oauth/revoke",
				strings.NewReader(test.form.Encode()),
			)
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("revoke empty token status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			var response oauthProtocolErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode OAuth error response: %v", err)
			}
			if response.Code != "invalid_request" {
				t.Fatalf("revoke empty token error = %q, want invalid_request", response.Code)
			}
		})
	}
}
