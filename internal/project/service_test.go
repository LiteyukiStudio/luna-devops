package project

import (
	"errors"
	"testing"
)

func TestCreateRejectsInvalidInputBeforeDatabaseAccess(t *testing.T) {
	service := NewService(nil)
	if _, err := service.Create(t.Context(), "usr_test", CreateInput{Identifier: "invalid value", Name: "Project"}); !errors.Is(err, ErrIdentifierInvalid) {
		t.Fatalf("invalid identifier error = %v", err)
	}
	if _, err := service.Create(t.Context(), "usr_test", CreateInput{Identifier: "valid-project", Name: "  "}); !errors.Is(err, ErrInputInvalid) {
		t.Fatalf("empty name error = %v", err)
	}
}

func TestProjectIdentifierConflictReflectsDeletionLifecycle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status   string
		expected error
	}{
		{status: "active", expected: ErrIdentifierExists},
		{status: "deleting", expected: ErrIdentifierDeleteInProgress},
		{status: "delete_failed", expected: ErrIdentifierDeleteFailed},
	}
	for _, test := range tests {
		if actual := projectIdentifierConflict(test.status); !errors.Is(actual, test.expected) {
			t.Fatalf("status %q error = %v, want %v", test.status, actual, test.expected)
		}
	}
}
