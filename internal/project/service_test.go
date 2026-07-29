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
