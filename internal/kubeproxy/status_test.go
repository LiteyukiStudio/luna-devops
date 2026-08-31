package kubeproxy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStatusErrorStringDoesNotLeakCause(t *testing.T) {
	err := Unauthorized(errors.New("Bearer credential-secret"))
	if strings.Contains(err.Error(), "credential-secret") || err.Error() != CodeUnauthorized {
		t.Fatalf("status error leaked its cause: %q", err.Error())
	}
}

func TestWriteStatusSetsNativeRetryAndAuthenticationHeaders(t *testing.T) {
	unauthorized := httptest.NewRecorder()
	WriteStatus(unauthorized, Unauthorized(errors.New("invalid")))
	if unauthorized.Code != http.StatusUnauthorized || unauthorized.Header().Get("WWW-Authenticate") != "Bearer" || !strings.Contains(unauthorized.Body.String(), `"kind":"Status"`) {
		t.Fatalf("unexpected unauthorized status: %d %#v %s", unauthorized.Code, unauthorized.Header(), unauthorized.Body.String())
	}
	rateLimited := httptest.NewRecorder()
	WriteStatus(rateLimited, RateLimited(errors.New("limit")))
	if rateLimited.Code != http.StatusTooManyRequests || rateLimited.Header().Get("Retry-After") != "1" {
		t.Fatalf("unexpected rate-limit status: %d %#v", rateLimited.Code, rateLimited.Header())
	}
}
