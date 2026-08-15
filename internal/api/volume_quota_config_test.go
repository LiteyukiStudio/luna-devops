package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func TestManagedVolumeQuotaConfigDefinitionAndValidation(t *testing.T) {
	t.Parallel()
	definition := configDefinitionByKey(volume.ProjectManagedCapacityLimitConfigKey)
	if definition == nil || definition.Type != "number" || definition.Default != "0" || definition.Public {
		t.Fatalf("managed volume quota definition = %#v", definition)
	}
	for _, valid := range []any{0, "0", 1024, "1048576"} {
		values, err := validateConfigValues(map[string]any{volume.ProjectManagedCapacityLimitConfigKey: valid})
		if err != nil || strings.TrimSpace(values[volume.ProjectManagedCapacityLimitConfigKey]) == "" {
			t.Errorf("valid managed volume quota %v: values=%#v err=%v", valid, values, err)
		}
	}
	for _, invalid := range []any{-1, 1.5, "invalid", 1048577} {
		if _, err := validateConfigValues(map[string]any{volume.ProjectManagedCapacityLimitConfigKey: invalid}); err == nil {
			t.Errorf("invalid managed volume quota %v was accepted", invalid)
		}
	}
}

func TestManagedVolumeBillingAdmissionDoesNotUseOptionalDeployFlag(t *testing.T) {
	db := authIntegrationDB(t)
	if err := db.AutoMigrate(&model.UserWallet{}); err != nil {
		t.Fatalf("migrate managed volume billing models: %v", err)
	}
	project := model.Project{
		ID: "prj_managed_volume_billing", Identifier: "managed-volume-billing", Name: "Managed volume billing",
		NamespaceStrategy: "project", BillingOwnerUserID: "usr_managed_volume_billing",
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create managed volume billing project: %v", err)
	}
	handlers := &Handlers{db: db, configs: &configCache{values: map[string]string{
		"billing.blockDeployChangesWhenInsufficient": "false",
	}}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/volumes", nil)
	if handlers.ensureBillingAllowsManagedVolumeChange(ctx, project.ID) {
		t.Fatal("managed volume admission accepted a zero billing-owner balance")
	}
	if recorder.Code != http.StatusPaymentRequired || !strings.Contains(recorder.Body.String(), "billing.insufficient_balance") {
		t.Fatalf("managed volume billing response = %d %s", recorder.Code, recorder.Body.String())
	}
	if err := db.Model(&model.UserWallet{}).Where("user_id = ?", project.BillingOwnerUserID).
		Update("balance_credits", decimal.NewFromInt(1)).Error; err != nil {
		t.Fatalf("fund managed volume billing owner: %v", err)
	}
	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/volumes", nil)
	if !handlers.ensureBillingAllowsManagedVolumeChange(ctx, project.ID) {
		t.Fatalf("managed volume admission rejected a positive balance: %d %s", recorder.Code, recorder.Body.String())
	}
}
