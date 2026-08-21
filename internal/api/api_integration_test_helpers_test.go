package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newAPIIntegrationContext(method, path string, body any, sessionToken string) (*httptest.ResponseRecorder, *gin.Context) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	ctx.Request = httptest.NewRequest(method, path, bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	return recorder, ctx
}

func jsonString(t *testing.T, data []byte, key string) string {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	value, _ := body[key].(string)
	return value
}
