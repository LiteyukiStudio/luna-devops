package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
)

func TestVolumeTransferMutationsStopBeforeContentWhenBalanceIsInsufficient(t *testing.T) {
	db := authIntegrationDB(t)
	if err := db.AutoMigrate(&model.UserWallet{}, &model.VolumeTransfer{}); err != nil {
		t.Fatalf("migrate volume billing models: %v", err)
	}
	user := model.User{
		ID: "usr_volume_billing", Email: "volume-billing@example.com", Name: "Volume billing",
		Role: authz.PlatformRoleAdmin, Language: "zh-CN", Password: "hash",
	}
	project := model.Project{
		ID: "prj_volume_billing", Identifier: "volume-billing", Name: "Volume billing",
		NamespaceStrategy: "isolated", BillingOwnerUserID: user.ID,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	transfer := model.VolumeTransfer{
		ID: "vtx_demo", ProjectID: project.ID, ProjectVolumeID: "pvol_demo",
		Direction: model.VolumeTransferDirectionExport, Format: model.VolumeTransferFormatTarGZ,
		State: model.VolumeTransferStateFailed, ActorID: user.ID,
	}
	if err := db.Create(&transfer).Error; err != nil {
		t.Fatalf("create transfer: %v", err)
	}
	handlers := &Handlers{db: db, mode: "production", configs: &configCache{values: map[string]string{
		"security.stepUpMfa.enabled":                 "false",
		"billing.blockDeployChangesWhenInsufficient": "true",
	}}, volumes: volume.NewGormService(db)}

	for _, test := range []struct {
		name   string
		method func(*gin.Context)
	}{
		{name: "import", method: handlers.CreateVolumeImport},
		{name: "export", method: handlers.CreateVolumeExport},
		{name: "retry", method: handlers.RetryVolumeTransfer},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+project.ID+"/volume-transfers/vtx_demo", nil)
			ctx.Params = gin.Params{{Key: "projectId", Value: project.ID}, {Key: "volumeId", Value: "pvol_demo"}, {Key: "transferId", Value: "vtx_demo"}}
			ctx.Set(currentUserContextKey, user)

			test.method(ctx)

			if recorder.Code != http.StatusPaymentRequired || !strings.Contains(recorder.Body.String(), "billing.insufficient_balance") {
				t.Fatalf("response=%d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestVolumeTransferBillingGuardKeepsAdministratorOptOut(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj_demo/volume-transfers", nil)
	handlers := &Handlers{configs: &configCache{values: map[string]string{
		"billing.blockDeployChangesWhenInsufficient": "false",
	}}}
	if !handlers.ensureBillingAllowsDeployChange(ctx, "prj_demo") {
		t.Fatal("disabled billing guard unexpectedly blocked the transfer")
	}
}
