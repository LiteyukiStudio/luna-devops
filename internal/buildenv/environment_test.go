package buildenv

import "testing"

func TestApplyUsesLastScopeAndPreservesSecretKind(t *testing.T) {
	snapshot := NewSnapshot()
	Apply(&snapshot, `{"SHARED":"global","GLOBAL_ONLY":"global"}`, `{"TOKEN":"secret-id:global"}`)
	Apply(&snapshot, `{"SHARED":"project","TOKEN":"public-project"}`, `{}`)
	Apply(&snapshot, `{}`, `{"SHARED":"secret-id:application"}`)
	Apply(&snapshot, `{"SHARED":"deployment"}`, `{"TARGET_TOKEN":"secret-id:deployment"}`)

	if snapshot.Variables["SHARED"] != "deployment" {
		t.Fatalf("expected deployment override, got %#v", snapshot.Variables)
	}
	if _, exists := snapshot.SecretRefs["SHARED"]; exists {
		t.Fatal("public deployment value must replace lower-scope secret")
	}
	if snapshot.Variables["TOKEN"] != "public-project" {
		t.Fatal("public project value must replace lower-scope secret")
	}
	if snapshot.SecretRefs["TARGET_TOKEN"] != "secret-id:deployment" {
		t.Fatal("deployment secret ref was not retained")
	}
}
