package deploymentapi

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

func TestApplyExactDeploymentTargetStageFilter(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test password=test dbname=test port=1 sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/deployment-targets?stage=prod", nil)

	statement := applyExactDeploymentTargetStageFilter(ctx, db.Model(&model.DeploymentTarget{})).Find(&[]model.DeploymentTarget{}).Statement
	if !strings.Contains(statement.SQL.String(), "stage = $1") {
		t.Fatalf("query does not use an exact deployment stage filter: %s", statement.SQL.String())
	}
	if len(statement.Vars) != 1 || statement.Vars[0] != "prod" {
		t.Fatalf("stage variables = %#v", statement.Vars)
	}
}
