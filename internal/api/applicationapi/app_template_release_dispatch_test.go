package applicationapi

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestMarkTemplateReleaseDispatchFailedGuardsTerminalReleases(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "app_template_release_dispatch_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Release{})
		},
	})
	pending := model.Release{ID: "rel_pending", ProjectID: "prj", ApplicationID: "app", DeploymentTargetID: "dplt", Status: "pending", Revision: 1}
	// Worker 已经推进到终态的发布不得被投递失败路径覆盖。
	succeeded := model.Release{ID: "rel_succeeded", ProjectID: "prj", ApplicationID: "app", DeploymentTargetID: "dplt_other", Status: "succeeded", Revision: 1}
	for _, release := range []*model.Release{&pending, &succeeded} {
		if err := db.Create(release).Error; err != nil {
			t.Fatalf("create release: %v", err)
		}
	}

	if err := markTemplateReleaseDispatchFailed(db, &pending); err != nil {
		t.Fatalf("markTemplateReleaseDispatchFailed(pending) error = %v", err)
	}
	if pending.Status != "failed" || pending.FailureCode != model.ReleaseFailureDispatchFailed || pending.FinishedAt == nil {
		t.Fatalf("pending release after dispatch failure = %#v", pending)
	}

	stale := succeeded
	stale.Status = "pending"
	if err := markTemplateReleaseDispatchFailed(db, &stale); err != nil {
		t.Fatalf("markTemplateReleaseDispatchFailed(succeeded) error = %v", err)
	}
	if stale.Status != "succeeded" || stale.FailureCode != "" {
		t.Fatalf("terminal release was overwritten: %#v", stale)
	}
}
