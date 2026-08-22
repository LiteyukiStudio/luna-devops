package api

import (
	"errors"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/resourcepolicy"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestCanUseRuntimeClusterForProject(t *testing.T) {
	user := model.User{ID: "usr_1", Role: authz.PlatformRoleUser}
	admin := model.User{ID: "usr_admin", Role: authz.PlatformRoleAdmin}

	cases := []struct {
		name            string
		user            model.User
		cluster         model.RuntimeCluster
		projectID       string
		boundProjectIDs []string
		want            bool
	}{
		{
			name:      "global cluster is usable by project members",
			user:      user,
			cluster:   model.RuntimeCluster{Scope: "global"},
			projectID: "prj_1",
			want:      true,
		},
		{
			name:            "project cluster bound to current project is usable",
			user:            user,
			cluster:         model.RuntimeCluster{Scope: "project"},
			projectID:       "prj_1",
			boundProjectIDs: []string{"prj_1"},
			want:            true,
		},
		{
			name:            "project cluster not bound to current project is rejected",
			user:            user,
			cluster:         model.RuntimeCluster{Scope: "project"},
			projectID:       "prj_1",
			boundProjectIDs: []string{"prj_2"},
			want:            false,
		},
		{
			name:      "own user cluster is usable",
			user:      user,
			cluster:   model.RuntimeCluster{Scope: "user", OwnerRef: "usr_1"},
			projectID: "prj_1",
			want:      true,
		},
		{
			name:      "another user cluster is rejected",
			user:      user,
			cluster:   model.RuntimeCluster{Scope: "user", OwnerRef: "usr_2"},
			projectID: "prj_1",
			want:      false,
		},
		{
			name:      "platform admin bypasses scope",
			user:      admin,
			cluster:   model.RuntimeCluster{Scope: "user", OwnerRef: "usr_2"},
			projectID: "prj_1",
			want:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canUseRuntimeClusterForProject(tc.user, tc.cluster, tc.projectID, tc.boundProjectIDs); got != tc.want {
				t.Fatalf("canUseRuntimeClusterForProject() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRuntimeClusterResourcePolicyDefaultsAndExplicitZero(t *testing.T) {
	defaults := runtimeClusterResourcePolicy(runtimeClusterInput{})
	if defaults != resourcepolicy.Default() {
		t.Fatalf("defaults = %#v", defaults)
	}
	zero := 0
	requestOnly := 50
	policy := runtimeClusterResourcePolicy(runtimeClusterInput{
		CPURequestPercent: &requestOnly, MemoryRequestPercent: &zero,
		CPULimitPercent: &zero, MemoryLimitPercent: &zero,
	})
	if err := policy.Validate(); err != nil || policy.CPURequestPercent != 50 || policy.CPULimitPercent != 0 {
		t.Fatalf("request-only policy = %#v, %v", policy, err)
	}
	limit := 25
	request := 30
	invalid := runtimeClusterResourcePolicy(runtimeClusterInput{CPURequestPercent: &request, CPULimitPercent: &limit})
	if err := invalid.Validate(); !errors.Is(err, resourcepolicy.ErrInvalidPolicy) {
		t.Fatalf("invalid policy error = %v", err)
	}
}

func TestRuntimeClusterExplicitZeroPolicyPersistsOnCreate(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "runtime_cluster_zero_policy_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.RuntimeCluster{})
		},
	})
	cluster := model.RuntimeCluster{
		ID: "clu_zero_policy", Name: "Zero Policy", Type: "kubernetes", Scope: "global",
		CPURequestPercent: 0, MemoryRequestPercent: 0, CPULimitPercent: 0, MemoryLimitPercent: 0,
	}
	if err := db.Create(&cluster).Error; err != nil {
		t.Fatalf("create runtime cluster: %v", err)
	}
	var persisted model.RuntimeCluster
	if err := db.First(&persisted, "id = ?", cluster.ID).Error; err != nil {
		t.Fatalf("read runtime cluster: %v", err)
	}
	if persisted.CPURequestPercent != 0 || persisted.MemoryRequestPercent != 0 || persisted.CPULimitPercent != 0 || persisted.MemoryLimitPercent != 0 {
		t.Fatalf("persisted runtime cluster policy = %#v", persisted)
	}
}
