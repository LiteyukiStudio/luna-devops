package platformevent

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestRecordReplayKeepsFirstActorAndResourceOwnerByEventID(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "platform_event_resource_owner_test",
		Migrate:      func(db *gorm.DB) error { return db.AutoMigrate(&model.PlatformEvent{}) },
	})
	service := Service{DB: db}
	first, created, err := service.Record(t.Context(), RecordInput{
		ID: "evt_resource_owner_replay", Type: "build.failed",
		ActorID: "usr_actor", ResourceOwnerUserID: "usr_owner",
		Detail: map[string]any{"id": "evt_resource_owner_replay", "type": "build.failed"},
	})
	if err != nil || !created {
		t.Fatalf("record first event created=%t err=%v", created, err)
	}
	replayed, created, err := service.Record(t.Context(), RecordInput{
		ID: "evt_resource_owner_replay", Type: "release.failed",
		ActorID: "usr_spoofed_actor", ResourceOwnerUserID: "usr_spoofed_owner",
		Detail: map[string]any{"id": "evt_resource_owner_replay", "type": "release.failed"},
	})
	if err != nil || created {
		t.Fatalf("replay event created=%t err=%v", created, err)
	}
	if replayed.ID != first.ID || replayed.Type != first.Type || replayed.ActorID != first.ActorID || replayed.ResourceOwnerUserID != first.ResourceOwnerUserID {
		t.Fatalf("replay returned non-authoritative event: %#v", replayed)
	}
}
