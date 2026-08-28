package notification

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestPersonalRecipientUserIDsDeduplicatesActorAndOwner(t *testing.T) {
	if got := PersonalRecipientUserIDs(" usr_actor ", "usr_owner"); len(got) != 2 || got[0] != "usr_actor" || got[1] != "usr_owner" {
		t.Fatalf("recipient ids = %#v", got)
	}
	if got := PersonalRecipientUserIDs("usr_same", " usr_same "); len(got) != 1 || got[0] != "usr_same" {
		t.Fatalf("deduplicated recipient ids = %#v", got)
	}
	if got := PersonalRecipientUserIDs("", " "); len(got) != 0 {
		t.Fatalf("empty recipient ids = %#v", got)
	}
}

func TestPersonalNotificationFanoutUsesActorAndResourceOwnerPolicies(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_visibility_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.User{},
				&model.ProjectMember{},
				&model.PlatformEvent{},
				&model.UserNotificationPreference{},
				&model.NotificationChannel{},
				&model.NotificationRule{},
				&model.NotificationDelivery{},
			)
		},
	})
	users := []model.User{
		{ID: "usr_visibility_actor", Email: "actor@example.com", Name: "Actor", Role: authz.PlatformRoleUser},
		{ID: "usr_visibility_owner", Email: "owner@example.com", Name: "Owner", Role: authz.PlatformRoleUser},
		{ID: "usr_visibility_outsider", Email: "outsider@example.com", Name: "Outsider", Role: authz.PlatformRoleUser},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	projectID := "prj_personal_visibility"
	members := []model.ProjectMember{
		{ID: "pmem_visibility_actor", ProjectID: projectID, UserID: users[0].ID, Role: authz.ProjectRoleDeveloper},
		{ID: "pmem_visibility_owner", ProjectID: projectID, UserID: users[1].ID, Role: authz.ProjectRoleOwner},
	}
	if err := db.Create(&members).Error; err != nil {
		t.Fatalf("create project members: %v", err)
	}
	if err := db.Model(&model.UserNotificationPreference{}).Create(map[string]any{
		"user_id":          users[1].ID,
		"email_enabled":    false,
		"event_types_json": EncodeStringList([]string{"build.failed"}),
	}).Error; err != nil {
		t.Fatalf("create owner preference: %v", err)
	}
	channels := []model.NotificationChannel{
		{ID: "nch_visibility_actor", OwnerUserID: users[0].ID, Name: "actor", AdapterKind: AdapterKindWebhook, ConfigJSON: `{}`, SecretRefsJSON: `{}`, Enabled: true},
		{ID: "nch_visibility_owner", OwnerUserID: users[1].ID, Name: "owner", AdapterKind: AdapterKindWebhook, ConfigJSON: `{}`, SecretRefsJSON: `{}`, Enabled: true},
		{ID: "nch_visibility_outsider", OwnerUserID: users[2].ID, Name: "outsider", AdapterKind: AdapterKindWebhook, ConfigJSON: `{}`, SecretRefsJSON: `{}`, Enabled: true},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatalf("create personal channels: %v", err)
	}

	service := Service{DB: db}
	deliveries, err := service.Emit(t.Context(), Event{
		ID:                  "evt_personal_visibility",
		Type:                "build.failed",
		Severity:            SeverityError,
		Project:             EntityRef{ID: projectID},
		Actor:               ActorContext{ID: users[0].ID},
		ResourceOwnerUserID: users[1].ID,
		Message:             "build failed",
	})
	if err != nil {
		t.Fatalf("emit personal visibility event: %v", err)
	}
	got := make(map[string]map[string]bool)
	for _, delivery := range deliveries {
		if got[delivery.RecipientUserID] == nil {
			got[delivery.RecipientUserID] = make(map[string]bool)
		}
		got[delivery.RecipientUserID][delivery.ChannelID] = true
	}
	if len(got) != 2 || !got[users[0].ID][UserEmailChannelID] || !got[users[0].ID][channels[0].ID] {
		t.Fatalf("actor deliveries = %#v", got[users[0].ID])
	}
	if got[users[1].ID][UserEmailChannelID] || !got[users[1].ID][channels[1].ID] {
		t.Fatalf("owner deliveries = %#v", got[users[1].ID])
	}
	if len(got[users[2].ID]) != 0 {
		t.Fatalf("outsider deliveries = %#v", got[users[2].ID])
	}
}

func TestPersonalRecipientPolicyRequiresParticipantAndCurrentProjectAccess(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_policy_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.User{}, &model.ProjectMember{}, &model.UserNotificationPreference{})
		},
	})
	users := []model.User{
		{ID: "usr_policy_actor", Email: "actor@example.com", Name: "Actor", Role: authz.PlatformRoleUser},
		{ID: "usr_policy_admin", Email: "admin@example.com", Name: "Admin", Role: authz.PlatformRoleAdmin},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create policy users: %v", err)
	}
	event := model.PlatformEvent{ID: "evt_policy", Type: "build.failed", ProjectID: "prj_policy", ActorID: users[0].ID}
	actorPolicy, code, err := LoadPersonalRecipientPolicy(t.Context(), db, users[0].ID)
	if err != nil || code != "" {
		t.Fatalf("load actor policy code=%q err=%v", code, err)
	}
	if code, err = actorPolicy.CheckEvent(t.Context(), db, event); err != nil || code != PersonalProjectAccessRevokedCode {
		t.Fatalf("actor without project access code=%q err=%v", code, err)
	}
	if err := db.Create(&model.ProjectMember{ID: "pmem_policy_actor", ProjectID: event.ProjectID, UserID: users[0].ID, Role: authz.ProjectRoleViewer}).Error; err != nil {
		t.Fatalf("create policy member: %v", err)
	}
	if code, err = actorPolicy.CheckEvent(t.Context(), db, event); err != nil || code != "" {
		t.Fatalf("actor with project access code=%q err=%v", code, err)
	}
	adminPolicy, code, err := LoadPersonalRecipientPolicy(t.Context(), db, users[1].ID)
	if err != nil || code != "" {
		t.Fatalf("load admin policy code=%q err=%v", code, err)
	}
	if code, err = adminPolicy.CheckEvent(t.Context(), db, event); err != nil || code != PersonalRecipientNotRelatedCode {
		t.Fatalf("unrelated platform admin code=%q err=%v", code, err)
	}
	event.ResourceOwnerUserID = users[1].ID
	if code, err = adminPolicy.CheckEvent(t.Context(), db, event); err != nil || code != "" {
		t.Fatalf("participating platform admin code=%q err=%v", code, err)
	}
}
