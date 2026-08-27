package worker

import (
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestAppendDeploymentHookRunLogRedactsSensitiveContent(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "deployment_hook_log_redaction_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.HookRunLog{})
		},
	})
	runner := &Runner{db: db}
	run := model.HookRun{ID: "hrun_redaction", ProjectID: "prj_redaction"}
	content := strings.Join([]string{
		"Authorization: Bearer bearer-secret",
		"token=query-secret",
		"plain configured secret is runtime-secret",
	}, "\n")

	runner.appendHookRunLog(t.Context(), run, content, []string{"runtime-secret"})

	var stored model.HookRunLog
	if err := db.First(&stored, "hook_run_id = ?", run.ID).Error; err != nil {
		t.Fatalf("load hook run log: %v", err)
	}
	for _, secret := range []string{"bearer-secret", "query-secret", "runtime-secret"} {
		if strings.Contains(stored.Content, secret) {
			t.Fatalf("stored hook log contains secret %q: %s", secret, stored.Content)
		}
	}
	if count := strings.Count(stored.Content, redactedLogValue); count != 3 {
		t.Fatalf("redacted marker count = %d, want 3: %s", count, stored.Content)
	}
}

func TestDecodedFileContentsReturnsResolvedSecretFileValues(t *testing.T) {
	got := decodedFileContents(`[{"path":"/run/secret","content":"file-secret"}]`)
	if len(got) != 1 || got[0] != "file-secret" {
		t.Fatalf("decoded file contents = %#v", got)
	}
}
