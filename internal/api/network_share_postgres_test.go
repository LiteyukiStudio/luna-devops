package api

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/repository"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestNetworkShareLifecyclePostgres(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	db := aiProjectScopeIntegrationDB(t)

	admin := model.User{ID: "usr_share_admin", Email: "share-admin@example.test", Name: "Admin", Role: authz.PlatformRoleAdmin}
	provider := model.User{ID: "usr_share_provider", Email: "share-provider@example.test", Name: "Provider", Role: authz.PlatformRoleUser}
	consumer := model.User{ID: "usr_share_consumer", Email: "share-consumer@example.test", Name: "Consumer", Role: authz.PlatformRoleUser}
	db1 := model.Project{ID: "prj_share_db", Identifier: "share-db", Name: "Database", KubernetesNamespace: "share-db", DeleteStatus: "active"}
	web := model.Project{ID: "prj_share_web", Identifier: "share-web", Name: "Web", KubernetesNamespace: "share-web", DeleteStatus: "active"}
	vault := model.Project{ID: "prj_share_vault", Identifier: "share-vault", Name: "Vault", KubernetesNamespace: "share-vault", DeleteStatus: "active"}
	cluster := model.RuntimeCluster{ID: "clu_share_default", Name: "Default", Scope: "global", IsDefault: true}
	seed := []any{
		&admin, &provider, &consumer, &db1, &web, &vault, &cluster,
		&model.ProjectMember{ID: "prjm_share_db", ProjectID: db1.ID, UserID: provider.ID, Role: authz.ProjectRoleOwner},
		&model.ProjectMember{ID: "prjm_share_vault", ProjectID: vault.ID, UserID: provider.ID, Role: authz.ProjectRoleOwner},
		&model.ProjectMember{ID: "prjm_share_web", ProjectID: web.ID, UserID: consumer.ID, Role: authz.ProjectRoleOwner},
		&model.DeploymentTarget{ID: "dplt_share_pg", ProjectID: db1.ID, ApplicationID: "app_share_pg", Name: "prod", Stage: "prod", KubernetesName: "pg"},
		&model.DeploymentTarget{ID: "dplt_share_web", ProjectID: web.ID, ApplicationID: "app_share_web", Name: "prod", Stage: "prod"},
		&model.DeploymentTarget{ID: "dplt_share_vault", ProjectID: vault.ID, ApplicationID: "app_share_vault", Name: "prod", Stage: "prod", KubernetesName: "vault"},
		&model.NetworkEgressException{ID: "nex_share_vault", RuntimeClusterID: cluster.ID, Name: "idc", CIDR: "10.9.0.0/16", Protocol: "tcp", Reason: "idc",
			Grants: []model.NetworkEgressExceptionGrant{{ID: "nexg_share_vault", ProjectID: vault.ID}}},
	}
	for _, value := range seed {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}
	handlers := &Handlers{db: db, projects: repository.NewProjectRepository(db)}
	handlers.domains = newDomainHandlers(handlers)
	project, runtime := handlers.domains.project, handlers.domains.runtime
	projectParams := func(projectID string, shareID string) gin.Params {
		params := gin.Params{{Key: "projectId", Value: projectID}}
		if shareID != "" {
			params = append(params, gin.Param{Key: "shareId", Value: shareID})
		}
		return params
	}
	create := func(user model.User, projectID, targetID, consumerIdentifier string) (int, string, map[string]any) {
		recorder := egressExceptionRequest(t, project.CreateProjectNetworkShare, user, http.MethodPost, projectParams(projectID, ""),
			`{"deploymentTargetId":"`+targetID+`","consumerProjectIdentifier":"`+consumerIdentifier+`"}`)
		body := map[string]any{}
		_ = json.Unmarshal(recorder.Body.Bytes(), &body)
		return recorder.Code, errorCode(t, recorder), body
	}

	t.Run("provider validation", func(t *testing.T) {
		cases := []struct {
			name, target, consumer, code string
			status                       int
		}{
			{"target of another project", "dplt_share_web", "share-web", "network_share.target_invalid", http.StatusBadRequest},
			{"sharing with itself", "dplt_share_pg", "share-db", "network_share.consumer_invalid", http.StatusBadRequest},
			{"unknown consumer", "dplt_share_pg", "missing", "network_share.consumer_invalid", http.StatusBadRequest},
		}
		for _, test := range cases {
			status, code, _ := create(provider, db1.ID, test.target, test.consumer)
			if status != test.status || code != test.code {
				t.Fatalf("%s: %d %s", test.name, status, code)
			}
		}
		if status, _, _ := create(consumer, db1.ID, "dplt_share_pg", "share-web"); status != http.StatusForbidden {
			t.Fatalf("non-members must not share a project's service: %d", status)
		}
	})

	var shareID string
	t.Run("consumer acceptance activates the share on both sides", func(t *testing.T) {
		status, _, body := create(provider, db1.ID, "dplt_share_pg", "share-web")
		if status != http.StatusCreated || body["status"] != "pending_acceptance" || body["approvalRequired"] != false || body["address"] != "pg.share-db.svc" {
			t.Fatalf("create = %d %#v", status, body)
		}
		shareID = body["id"].(string)
		if status, code, _ := create(provider, db1.ID, "dplt_share_pg", "share-web"); status != http.StatusConflict || code != "network_share.duplicate" {
			t.Fatalf("duplicate = %d %s", status, code)
		}
		assertSharePolicies(t, db, cluster, db1, web, false)
		// 提供方不能替使用方确认。
		recorder := egressExceptionRequest(t, project.AcceptProjectNetworkShare, provider, http.MethodPost, projectParams(db1.ID, shareID), "")
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("provider accept = %d", recorder.Code)
		}
		recorder = egressExceptionRequest(t, project.AcceptProjectNetworkShare, consumer, http.MethodPost, projectParams(web.ID, shareID), "")
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"active"`) {
			t.Fatalf("consumer accept = %d %s", recorder.Code, recorder.Body.String())
		}
		assertSharePolicies(t, db, cluster, db1, web, true)
		recorder = egressExceptionRequest(t, project.ListProjectNetworkShares, consumer, http.MethodGet, projectParams(web.ID, ""), "")
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("missing direction must be rejected: %d", recorder.Code)
		}
	})

	t.Run("providers holding egress exceptions need cluster maintainer approval", func(t *testing.T) {
		status, _, body := create(provider, vault.ID, "dplt_share_vault", "share-web")
		if status != http.StatusCreated || body["approvalRequired"] != true {
			t.Fatalf("create = %d %#v", status, body)
		}
		vaultShare := body["id"].(string)
		recorder := egressExceptionRequest(t, project.AcceptProjectNetworkShare, consumer, http.MethodPost, projectParams(web.ID, vaultShare), "")
		if !strings.Contains(recorder.Body.String(), `"status":"pending_approval"`) {
			t.Fatalf("accepted share must wait for approval: %s", recorder.Body.String())
		}
		assertSharePolicies(t, db, cluster, vault, web, false)
		clusterParams := gin.Params{{Key: "clusterId", Value: cluster.ID}, {Key: "shareId", Value: vaultShare}}
		recorder = egressExceptionRequest(t, runtime.ApproveRuntimeClusterNetworkShare, provider, http.MethodPost, clusterParams, "")
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("project owners must not approve their own share: %d", recorder.Code)
		}
		recorder = egressExceptionRequest(t, runtime.ListRuntimeClusterNetworkShareApprovals, admin, http.MethodGet, clusterParams[:1], "")
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), vaultShare) || strings.Contains(recorder.Body.String(), shareID) {
			t.Fatalf("approvals = %d %s", recorder.Code, recorder.Body.String())
		}
		recorder = egressExceptionRequest(t, runtime.ApproveRuntimeClusterNetworkShare, admin, http.MethodPost, clusterParams, "")
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"active"`) {
			t.Fatalf("approve = %d %s", recorder.Code, recorder.Body.String())
		}
		assertSharePolicies(t, db, cluster, vault, web, true)
		recorder = egressExceptionRequest(t, runtime.ApproveRuntimeClusterNetworkShare, admin, http.MethodPost, gin.Params{{Key: "clusterId", Value: cluster.ID}, {Key: "shareId", Value: shareID}}, "")
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("shares without approval requirement cannot be approved: %d", recorder.Code)
		}
	})

	t.Run("consumer leaving removes the share and is audited", func(t *testing.T) {
		recorder := egressExceptionRequest(t, project.DeleteProjectNetworkShare, consumer, http.MethodDelete, projectParams(web.ID, shareID), "")
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("delete = %d %s", recorder.Code, recorder.Body.String())
		}
		assertSharePolicies(t, db, cluster, db1, web, false)
		var logs []model.AuditLog
		db.Where("action like ?", "%network_share.%").Find(&logs)
		actions := map[string]bool{}
		for _, log := range logs {
			actions[log.Action] = true
		}
		for _, action := range []string{"project.network_share.create", "project.network_share.accept", "project.network_share.delete", "runtime_cluster.network_share.approve"} {
			if !actions[action] {
				t.Fatalf("missing audit %s in %#v", action, actions)
			}
		}
	})
}

// assertSharePolicies 校验提供方与使用方渲染结果中是否包含共享放行策略。
func assertSharePolicies(t *testing.T, db *gorm.DB, cluster model.RuntimeCluster, provider, consumer model.Project, want bool) {
	t.Helper()
	// peerNamespace 是共享策略指向的对端 Namespace：入站看来源，出站看目标。
	has := func(project model.Project, prefix string, peerNamespace string) bool {
		policies, err := runtimecluster.DesiredWorkloadNetworkPolicies(context.Background(), db, cluster, project.ID, project.KubernetesNamespace, []string{"192.0.2.10/32"})
		if err != nil {
			t.Fatalf("render %s: %v", project.ID, err)
		}
		for _, policy := range policies {
			if !strings.HasPrefix(policy.Name, prefix) {
				continue
			}
			for _, rule := range policy.Spec.Ingress {
				if slices.Contains(rule.From[0].NamespaceSelector.MatchExpressions[0].Values, peerNamespace) {
					return true
				}
			}
			for _, rule := range policy.Spec.Egress {
				if rule.To[0].NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] == peerNamespace {
					return true
				}
			}
		}
		return false
	}
	if got := has(provider, "luna-shared-ingress-", consumer.KubernetesNamespace); got != want {
		t.Fatalf("provider %s shared ingress = %t, want %t", provider.ID, got, want)
	}
	if got := has(consumer, "luna-shared-egress-", provider.KubernetesNamespace); got != want {
		t.Fatalf("consumer %s shared egress = %t, want %t", consumer.ID, got, want)
	}
}
