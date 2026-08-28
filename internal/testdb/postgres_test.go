package testdb

import "testing"

func TestIsolatedNameFitsPostgresIdentifierLimit(t *testing.T) {
	name := isolatedName("notification_resource_owner_migration_test")
	if len(name) > postgresIdentifierMaxBytes {
		t.Fatalf("isolated name length = %d, want at most %d: %q", len(name), postgresIdentifierMaxBytes, name)
	}
	if !identifierPrefixPattern.MatchString(name) {
		t.Fatalf("isolated name is not a valid identifier: %q", name)
	}
}

func TestIsolatedNameRemainsUniqueAfterPrefixTruncation(t *testing.T) {
	const prefix = "notification_resource_owner_migration_test"
	first := isolatedName(prefix)
	second := isolatedName(prefix)
	if first == second {
		t.Fatalf("isolated names are not unique: %q", first)
	}
}
