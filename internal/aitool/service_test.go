package aitool

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm/schema"
)

func TestListAppTemplatesSupportsSearchAndDoesNotExposeSecretDefaults(t *testing.T) {
	result, err := NewService(nil).Execute(context.Background(), Request{
		OperationID: "listAppTemplates",
		Arguments:   map[string]any{"query": "postgres"},
	})
	if err != nil {
		t.Fatalf("list app templates: %v", err)
	}
	value, ok := result.Value.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T", result.Value)
	}
	items, ok := value["items"].([]map[string]any)
	if !ok || len(items) == 0 {
		t.Fatalf("items = %#v", value["items"])
	}
	for _, item := range items {
		values, _ := item["values"].([]map[string]any)
		for _, definition := range values {
			if secret, _ := definition["secret"].(bool); secret {
				if _, exists := definition["default"]; exists {
					t.Fatal("secret template value must not expose its default")
				}
			}
		}
	}
}

func TestApplicationListColumnsMatchApplicationSchema(t *testing.T) {
	applicationSchema, err := schema.Parse(&model.Application{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse application schema: %v", err)
	}

	for _, column := range strings.Split(applicationListColumns, ", ") {
		if applicationSchema.LookUpField(column) == nil {
			t.Fatalf("AI listApplications selects unknown applications column %q", column)
		}
	}
	if strings.Contains(applicationListColumns, "description") {
		t.Fatal("AI listApplications must not select the removed applications.description column")
	}
}

func TestDatabaseResultClassifiesStorageFailure(t *testing.T) {
	_, err := databaseResult(Result{Value: "ignored"}, errors.New("driver detail must remain internal"))
	if !errors.Is(err, ErrStorage) {
		t.Fatalf("databaseResult error = %v, want ErrStorage", err)
	}
}

func TestTargetProjectComesOnlyFromBoundToolArguments(t *testing.T) {
	projectID, err := targetProjectID(Policy{ProjectAction: authz.ActionApplicationRead}, map[string]any{
		"projectId": " prj_selected ",
	})
	if err != nil || projectID != "prj_selected" {
		t.Fatalf("target = %q, error = %v", projectID, err)
	}
	if _, err := targetProjectID(Policy{ProjectAction: authz.ActionApplicationRead}, map[string]any{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing project error = %v", err)
	}
	if _, err := targetProjectID(Policy{}, map[string]any{"projectId": "prj_page"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("platform tool project error = %v", err)
	}
}
