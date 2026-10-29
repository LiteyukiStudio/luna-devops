package transport

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDeliveryIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name   string
		header string
		want   string
		valid  bool
	}{
		{name: "empty", valid: true},
		{name: "scoped", header: "build-request-0001", want: "build.trigger:build-request-0001", valid: true},
		{name: "too short", header: "short", valid: false},
		{name: "too long", header: strings.Repeat("x", 129), valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest("POST", "/", nil)
			if test.header != "" {
				ctx.Request.Header.Set("Idempotency-Key", test.header)
			}
			got, valid := DeliveryIdempotencyKey(ctx, "build.trigger")
			if got != test.want || valid != test.valid {
				t.Fatalf("DeliveryIdempotencyKey() = (%q, %v), want (%q, %v)", got, valid, test.want, test.valid)
			}
		})
	}
}

func TestDeliveryRequestHashIsStableForEquivalentJSONObjects(t *testing.T) {
	left := DeliveryRequestHash(map[string]any{
		"projectId": "prj_1",
		"body":      map[string]any{"imageRef": "registry/app:one", "force": true},
	})
	right := DeliveryRequestHash(map[string]any{
		"body":      map[string]any{"force": true, "imageRef": "registry/app:one"},
		"projectId": "prj_1",
	})
	changed := DeliveryRequestHash(map[string]any{
		"projectId": "prj_1",
		"body":      map[string]any{"imageRef": "registry/app:two", "force": true},
	})

	if left == "" || left != right {
		t.Fatalf("equivalent delivery requests hashed differently: %q != %q", left, right)
	}
	if left == changed {
		t.Fatal("changed delivery request reused the same hash")
	}
}
