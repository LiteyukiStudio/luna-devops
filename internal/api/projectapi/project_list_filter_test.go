package projectapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestApplyExactProjectIdentifierFilter(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test password=test dbname=test port=1 sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/projects?identifier=xnn-api", nil)

	statement := applyExactProjectIdentifierFilter(ctx, db.Model(&model.Project{})).Find(&[]model.Project{}).Statement
	if !strings.Contains(statement.SQL.String(), "projects.identifier = $1") {
		t.Fatalf("query does not use an exact project identifier filter: %s", statement.SQL.String())
	}
	if len(statement.Vars) != 1 || statement.Vars[0] != "xnn-api" {
		t.Fatalf("identifier variables = %#v", statement.Vars)
	}
}
