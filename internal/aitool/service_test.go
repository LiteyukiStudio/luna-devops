package aitool

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"gorm.io/gorm/schema"
)

func TestListAppTemplatesReturnsSummariesWithoutValues(t *testing.T) {
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
		if _, exists := item["values"]; exists {
			t.Fatal("list summary must not embed full parameter values")
		}
		if _, exists := item["valueCount"]; !exists {
			t.Fatal("list summary should report parameter count")
		}
		if _, exists := item["dataVolumes"]; !exists {
			t.Fatal("list summary should expose typed data volume declarations")
		}
		for _, legacy := range []string{"dataRetentionEnabled", "dataCapacity", "dataMountPath"} {
			if _, exists := item[legacy]; exists {
				t.Fatalf("list summary exposes legacy volume field %s", legacy)
			}
		}
		if item["id"].(string) == "" || item["name"].(string) == "" {
			t.Fatal("list summary must keep identity fields")
		}
	}
}

func TestGetAppTemplateReturnsFullValuesAndHidesSecretDefaults(t *testing.T) {
	service := NewService(nil)
	list, err := service.Execute(context.Background(), Request{
		OperationID: "listAppTemplates",
		Arguments:   map[string]any{"query": "postgres"},
	})
	if err != nil {
		t.Fatalf("list app templates: %v", err)
	}
	items := list.Value.(map[string]any)["items"].([]map[string]any)
	id := items[0]["id"].(string)

	result, err := service.Execute(context.Background(), Request{
		OperationID: "getAppTemplate",
		Arguments:   map[string]any{"id": id},
	})
	if err != nil {
		t.Fatalf("get app template: %v", err)
	}
	detail := result.Value.(map[string]any)
	if _, exists := detail["dataVolumes"]; !exists {
		t.Fatal("template detail should expose typed data volume declarations")
	}
	for _, legacy := range []string{"dataRetentionEnabled", "dataCapacity", "dataMountPath"} {
		if _, exists := detail[legacy]; exists {
			t.Fatalf("template detail exposes legacy volume field %s", legacy)
		}
	}
	values, ok := detail["values"].([]map[string]any)
	if !ok {
		t.Fatalf("detail values = %#v", detail["values"])
	}
	for _, definition := range values {
		if secret, _ := definition["secret"].(bool); secret {
			if _, exists := definition["default"]; exists {
				t.Fatal("secret template value must not expose its default")
			}
		}
	}

	if _, err := service.Execute(context.Background(), Request{
		OperationID: "getAppTemplate",
		Arguments:   map[string]any{},
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing id error = %v", err)
	}
	if _, err := service.Execute(context.Background(), Request{
		OperationID: "getAppTemplate",
		Arguments:   map[string]any{"id": "nonexistent-template-id"},
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown template error = %v", err)
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

func TestAuthorizeActorFailsClosedWithoutDependencies(t *testing.T) {
	if NewService(nil).AuthorizeActor(t.Context(), "usr_1", "ses_1", "prj_1", Policy{ProjectAction: authz.ActionProjectRead}) {
		t.Fatal("authorization without storage or ProjectAuthorizer must fail closed")
	}
}

func TestProjectListOptionsDefaultToRelatedAndBoundedPagination(t *testing.T) {
	for _, platformAdmin := range []bool{false, true} {
		options, err := resolveProjectListOptions(map[string]any{}, platformAdmin)
		if err != nil {
			t.Fatalf("platformAdmin=%t default options error = %v", platformAdmin, err)
		}
		if options.Visibility != projectservice.ListVisibilityRelated || options.Page != 1 || options.PageSize != 20 {
			t.Fatalf("platformAdmin=%t default options = %#v", platformAdmin, options)
		}
	}

	options, err := resolveProjectListOptions(map[string]any{"visibility": "all", "page": float64(3), "pageSize": float64(100)}, true)
	if err != nil {
		t.Fatalf("admin all options error = %v", err)
	}
	if options.Visibility != projectservice.ListVisibilityAll || options.Page != 3 || options.PageSize != 100 {
		t.Fatalf("admin all options = %#v", options)
	}
}

func TestProjectListOptionsRejectUnauthorizedOrInvalidArguments(t *testing.T) {
	for name, testCase := range map[string]struct {
		arguments     map[string]any
		platformAdmin bool
		want          error
	}{
		"non-admin all":         {arguments: map[string]any{"visibility": "all"}, want: ErrForbidden},
		"unknown visibility":    {arguments: map[string]any{"visibility": "mine"}, platformAdmin: true, want: ErrInvalidInput},
		"non-string visibility": {arguments: map[string]any{"visibility": float64(1)}, platformAdmin: true, want: ErrInvalidInput},
		"zero page":             {arguments: map[string]any{"page": float64(0)}, want: ErrInvalidInput},
		"fractional page":       {arguments: map[string]any{"page": 1.5}, want: ErrInvalidInput},
		"oversized pageSize":    {arguments: map[string]any{"pageSize": float64(101)}, want: ErrInvalidInput},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := resolveProjectListOptions(testCase.arguments, testCase.platformAdmin); !errors.Is(err, testCase.want) {
				t.Fatalf("error = %v, want %v", err, testCase.want)
			}
		})
	}
}
