package projectapi

import (
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestNormalizeEgressAllowlist(t *testing.T) {
	got, err := normalizeEgressAllowlist("1.1.1.9/24  443,8443\n\n2606:4700::/32\n")
	if err != nil || got != "1.1.1.0/24 443,8443\n2606:4700::/32" {
		t.Fatalf("normalized = %q, %v", got, err)
	}
	for _, invalid := range []string{
		"0.0.0.0/0",
		"10.1.0.0/16",
		"169.254.169.254/32",
		"1.1.1.0/24 0",
		"1.1.1.0/24 443 extra",
		"not-a-cidr",
		// IPv4 映射的 IPv6 会被 Kubernetes 当作 IPv4 网段，必须拒绝。
		"::ffff:0.0.0.0/96",
		"::ffff:169.254.169.254/128",
		"::ffff:1.1.1.1/128 443",
	} {
		if _, err := normalizeEgressAllowlist(invalid); err == nil {
			t.Fatalf("allowlist %q must be rejected", invalid)
		}
	}
	// 包含私网段的公网大网段允许保存，私网部分由渲染器排除。
	if _, err := normalizeEgressAllowlist("0.0.0.0/1"); err != nil {
		t.Fatalf("wide public range rejected: %v", err)
	}
}

func TestProjectEgressExceptionsReadsGrantedRules(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "project_egress_exception_view_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.NetworkEgressException{}, &model.NetworkEgressExceptionGrant{})
		},
	})
	now := time.Now()
	exception := model.NetworkEgressException{ID: "nex_db", RuntimeClusterID: "clu_dc", Name: "db", CIDR: "192.168.31.10/32", Ports: "3306", Protocol: "tcp", Reason: "billing", CreatedAt: now, UpdatedAt: now,
		Grants: []model.NetworkEgressExceptionGrant{{ID: "nexg_1", ProjectID: "prj_pay", DeploymentTargetIDs: "dt_api,dt_job", IncludeBuilds: true, CreatedAt: now}}}
	if err := db.Create(&exception).Error; err != nil {
		t.Fatal(err)
	}
	got, err := projectEgressExceptions(db, "prj_pay")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].CIDR != "192.168.31.10/32" || got[0].RuntimeClusterID != "clu_dc" || len(got[0].DeploymentTargetIDs) != 2 || !got[0].IncludeBuilds {
		t.Fatalf("project egress exceptions = %#v", got)
	}
	if other, _ := projectEgressExceptions(db, "prj_other"); len(other) != 0 {
		t.Fatalf("exceptions must be scoped to granted projects: %#v", other)
	}
}
