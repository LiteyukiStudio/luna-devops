package worker

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestNotificationLinksPointToApplicationTabs(t *testing.T) {
	runner := NewRunner(nil, Options{PublicBaseURL: "https://devops.example.com/"})

	links := runner.notificationLinks("prj_1", "app_1", "deployments", "release")

	if links["project"] != "https://devops.example.com/projects/prj_1" {
		t.Fatalf("project link = %q", links["project"])
	}
	if links["application"] != "https://devops.example.com/projects/prj_1/apps/app_1" {
		t.Fatalf("application link = %q", links["application"])
	}
	if links["release"] != "https://devops.example.com/projects/prj_1/apps/app_1#tab=deployments" {
		t.Fatalf("release link = %q", links["release"])
	}
	if links["primary"] != links["release"] {
		t.Fatalf("primary link = %q, release link = %q", links["primary"], links["release"])
	}
}

func TestNotificationLinksStayEmptyWithoutPublicBaseURL(t *testing.T) {
	runner := NewRunner(nil, Options{})

	if links := runner.notificationLinks("prj_1", "app_1", "builds", "build"); links != nil {
		t.Fatalf("links = %#v", links)
	}
}

func TestNotificationBuildImageRefPrefersPersistedImage(t *testing.T) {
	run := model.BuildRun{
		ImageRef:         "registry.example.com/team/app:resolved",
		TargetRepository: "registry.example.com/team/app",
		TargetTag:        "fallback",
	}

	if got := (&Runner{}).notificationBuildImageRef(run); got != run.ImageRef {
		t.Fatalf("notificationBuildImageRef() = %q, want %q", got, run.ImageRef)
	}
}

func TestNotificationBuildImageRefFallsBackToBuildTarget(t *testing.T) {
	run := model.BuildRun{
		TargetRepository: "registry.example.com/team/app",
		TargetTag:        "{branchSlug}-{shortSha}",
		SourceBranch:     "feature/identifier-notifications",
		SourceCommit:     "1234567890abcdef",
	}

	want := "registry.example.com/team/app:feature-identifier-notifications-1234567890ab"
	if got := (&Runner{}).notificationBuildImageRef(run); got != want {
		t.Fatalf("notificationBuildImageRef() = %q, want %q", got, want)
	}
}

func TestNotificationHookActorUsesInternalBuildOrReleaseCreator(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "notification_hook_actor_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.User{}, &model.BuildRun{}, &model.Release{})
		},
	})
	users := []model.User{
		{ID: "usr_build", Email: "build-user@example.com", Name: "Build User"},
		{ID: "usr_release", Email: "release-user@example.com", Name: "Release User"},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	if err := db.Create(&model.BuildRun{
		ID:               "brn_hook_actor",
		ProjectID:        "prj_hook_actor",
		CreatedBy:        "usr_build",
		TriggeredByName:  "External Trigger",
		TriggeredByEmail: "external-trigger@example.com",
	}).Error; err != nil {
		t.Fatalf("create build run: %v", err)
	}
	if err := db.Create(&model.Release{
		ID:            "rel_hook_actor",
		ProjectID:     "prj_hook_actor",
		ApplicationID: "app_hook_actor",
		EnvironmentID: "env_hook_actor",
		ImageRef:      "example.invalid/app:test",
		CreatedBy:     "usr_release",
	}).Error; err != nil {
		t.Fatalf("create release: %v", err)
	}

	runner := &Runner{db: db}
	buildActor := runner.notificationHookActor(t.Context(), model.HookRun{
		ProjectID:  "prj_hook_actor",
		BuildRunID: "brn_hook_actor",
	})
	if buildActor.ID != "usr_build" {
		t.Fatalf("build hook actor ID = %q, want internal build creator", buildActor.ID)
	}
	if buildActor.Email != "external-trigger@example.com" {
		t.Fatalf("build hook display email = %q, want trigger metadata", buildActor.Email)
	}

	releaseActor := runner.notificationHookActor(t.Context(), model.HookRun{
		ProjectID: "prj_hook_actor",
		ReleaseID: "rel_hook_actor",
	})
	if releaseActor.ID != "usr_release" || releaseActor.Email != "release-user@example.com" {
		t.Fatalf("release hook actor = %#v, want internal release creator", releaseActor)
	}

	unknownActor := runner.notificationHookActor(t.Context(), model.HookRun{
		ProjectID:  "prj_hook_actor",
		BuildRunID: "brn_external",
	})
	if unknownActor.ID != "" || unknownActor.Email != "" {
		t.Fatalf("unknown hook actor = %#v, want empty", unknownActor)
	}
}
