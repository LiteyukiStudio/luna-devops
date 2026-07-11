package api

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func TestCanReadPlatformEventForUser(t *testing.T) {
	user := model.User{ID: "usr_current", Role: authz.PlatformRoleUser}
	memberEvent := model.PlatformEvent{ProjectID: "prj_allowed"}
	if !canReadPlatformEventForUser(user, memberEvent, []string{"prj_allowed"}) {
		t.Fatal("expected project member to read project event")
	}
	if canReadPlatformEventForUser(user, model.PlatformEvent{ProjectID: "prj_other"}, []string{"prj_allowed"}) {
		t.Fatal("expected cross-project event to be denied")
	}
	if !canReadPlatformEventForUser(user, model.PlatformEvent{ActorID: user.ID}, nil) {
		t.Fatal("expected actor to read their user-level event")
	}
	admin := model.User{ID: "usr_admin", Role: authz.PlatformRoleAdmin}
	if !canReadPlatformEventForUser(admin, model.PlatformEvent{ProjectID: "prj_other"}, nil) {
		t.Fatal("expected platform admin to read all events")
	}
}

func TestPlatformEventFilterValuesSupportsSingleRepeatedAndCommaSeparatedValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/events?projectId=legacy&projectIds=first&projectIds=second,third&projectIds=first", nil)

	values := platformEventFilterValues(ctx, "projectId", "projectIds")
	want := []string{"first", "second", "third", "legacy"}
	if len(values) != len(want) {
		t.Fatalf("filter values = %v, want %v", values, want)
	}
	for index := range want {
		if values[index] != want[index] {
			t.Fatalf("filter values = %v, want %v", values, want)
		}
	}
}

func TestParsePlatformEventTimeUsesInclusiveEndOfDay(t *testing.T) {
	parsed, ok := parsePlatformEventTime("2026-07-11", true)
	if !ok {
		t.Fatal("expected date to parse")
	}
	want := time.Date(2026, 7, 11, 23, 59, 59, int(time.Second-time.Nanosecond), time.UTC)
	if !parsed.Equal(want) {
		t.Fatalf("parsed end of day = %s, want %s", parsed, want)
	}
	if _, ok := parsePlatformEventTime("not-a-date", false); ok {
		t.Fatal("expected invalid date to be rejected")
	}
}
