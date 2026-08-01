package database

import (
	"strings"
	"testing"

	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
)

func TestInboxMigrationDefinesUserIsolationAndDedupIndexes(t *testing.T) {
	t.Parallel()
	data, err := sqlmigrations.FS.ReadFile("000060_inbox_messages.up.sql")
	if err != nil {
		t.Fatalf("read inbox migration: %v", err)
	}
	sql := string(data)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS inbox_messages",
		"recipient_user_id text NOT NULL",
		"params_json jsonb NOT NULL",
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_inbox_messages_dedup_key",
		"WHERE dedup_key IS NOT NULL",
		"CREATE TABLE IF NOT EXISTS inbox_action_requests",
		"row_version bigint NOT NULL DEFAULT 1",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("inbox migration is missing %q", fragment)
		}
	}
}
