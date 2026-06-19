package billing

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/shopspring/decimal"
)

func TestDeploymentTargetStorageGiBSumsDataVolumes(t *testing.T) {
	target := model.DeploymentTarget{
		DataRetentionEnabled: true,
		DataCapacity:         "1Gi",
		DataVolumes:          `[{"name":"app1","mountPath":"/data/app1","capacity":"20Gi"},{"name":"app2","mountPath":"/data/app2","capacity":"40Gi"}]`,
	}
	got := deploymentTargetStorageGiB(target)
	if !got.Equal(decimalFromInt(60)) {
		t.Fatalf("storage GiB = %s", got)
	}
}

func TestDeploymentTargetStorageGiBFallsBackToPrimaryCapacity(t *testing.T) {
	target := model.DeploymentTarget{DataRetentionEnabled: true, DataCapacity: "5Gi"}
	got := deploymentTargetStorageGiB(target)
	if !got.Equal(decimalFromInt(5)) {
		t.Fatalf("storage GiB = %s", got)
	}
}

func decimalFromInt(value int64) decimal.Decimal {
	return decimal.NewFromInt(value)
}
