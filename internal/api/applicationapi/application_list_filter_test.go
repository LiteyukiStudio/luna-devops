package applicationapi

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

func TestApplyExactApplicationIdentifierFilter(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test password=test dbname=test port=1 sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/applications?identifier=postgres-w8kt4h", nil)

	statement := applyExactApplicationIdentifierFilter(ctx, db.Model(&model.Application{})).Find(&[]model.Application{}).Statement
	if !strings.Contains(statement.SQL.String(), "identifier = $1") {
		t.Fatalf("query does not use an exact application identifier filter: %s", statement.SQL.String())
	}
	if len(statement.Vars) != 1 || statement.Vars[0] != "postgres-w8kt4h" {
		t.Fatalf("identifier variables = %#v", statement.Vars)
	}
}
