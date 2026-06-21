package billing

import (
	"testing"
	"time"

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

func TestDefaultRateRulesPreferGatewayTrafficOverRequestBilling(t *testing.T) {
	rules := defaultRateRuleByMeter()
	traffic, ok := rules["gateway.egress_gib"]
	if !ok {
		t.Fatal("expected gateway traffic billing rule")
	}
	if !traffic.Enabled {
		t.Fatal("expected gateway traffic billing to be enabled by default")
	}
	if traffic.Unit != "gib" {
		t.Fatalf("gateway traffic unit = %q", traffic.Unit)
	}
	requests, ok := rules["gateway.requests_1000"]
	if !ok {
		t.Fatal("expected gateway request count rule")
	}
	if requests.Enabled {
		t.Fatal("expected request count billing to be disabled by default")
	}
	if !requests.CreditsPerUnit.Equal(decimal.Zero) {
		t.Fatalf("request count price = %s", requests.CreditsPerUnit)
	}
}

func TestGatewayTrafficUsageResourceIDUsesMinuteWindow(t *testing.T) {
	periodStart := time.Date(2026, 6, 21, 10, 5, 30, 0, time.FixedZone("CST", 8*3600))
	got := gatewayTrafficUsageResourceID("gwr_demo", periodStart)
	if got != "gwr_demo:202606210205" {
		t.Fatalf("resource id = %q", got)
	}
}

func decimalFromInt(value int64) decimal.Decimal {
	return decimal.NewFromInt(value)
}
