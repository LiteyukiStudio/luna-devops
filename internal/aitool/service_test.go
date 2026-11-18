package aitool

import (
	"errors"
	"testing"
)

func TestExecuteRejectsMissingActorOrUnsupportedOperation(t *testing.T) {
	service := NewService()

	if _, err := service.Execute(t.Context(), Request{
		OperationID: "webSearch",
		UserID:      "",
		Arguments:   map[string]any{"query": "luna"},
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("missing user error = %v, want ErrForbidden", err)
	}

	for _, operationID := range []string{"getDashboard", "listProjects", "listAppTemplates", "unknownOperation"} {
		if _, err := service.Execute(t.Context(), Request{
			OperationID: operationID,
			UserID:      "usr_1",
			Arguments:   map[string]any{},
		}); !errors.Is(err, ErrForbidden) {
			t.Fatalf("Execute(%q) error = %v, want ErrForbidden", operationID, err)
		}
	}
}
