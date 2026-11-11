package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/repository"
	"github.com/gin-gonic/gin"
)

func egressExceptionRequest(t *testing.T, handler gin.HandlerFunc, user model.User, method string, params gin.Params, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, "/api/v1/runtime/clusters/x/egress-exceptions", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = params
	ctx.Set(currentUserContextKey, user)
	handler(ctx)
	// Gin 引擎在处理链结束后才写出仅设置了状态码的响应；直接调用 handler 时需要手动写出。
	ctx.Writer.WriteHeaderNow()
	return recorder
}

func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return body.Code
}

func TestEgressExceptionAuthorizationAndGrantsPostgres(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	db := aiProjectScopeIntegrationDB(t)

	admin := model.User{ID: "usr_egress_admin", Email: "egress-admin@example.test", Name: "Admin", Role: authz.PlatformRoleAdmin}
	owner := model.User{ID: "usr_egress_owner", Email: "egress-owner@example.test", Name: "Owner", Role: authz.PlatformRoleUser}
	stranger := model.User{ID: "usr_egress_stranger", Email: "egress-stranger@example.test", Name: "Stranger", Role: authz.PlatformRoleUser}
	pay := model.Project{ID: "prj_egress_pay", Identifier: "egress-pay", Name: "Pay", KubernetesNamespace: "egress-pay", DeleteStatus: "active"}
	other := model.Project{ID: "prj_egress_other", Identifier: "egress-other", Name: "Other", KubernetesNamespace: "egress-other", DeleteStatus: "active"}
	seed := []any{
		&admin, &owner, &stranger, &pay, &other,
		&model.ProjectMember{ID: "prjm_egress_owner", ProjectID: pay.ID, UserID: owner.ID, Role: authz.ProjectRoleOwner},
		&model.RuntimeCluster{ID: "clu_egress_global", Name: "Global", Scope: "global"},
		&model.RuntimeCluster{ID: "clu_egress_user", Name: "Personal", Scope: "user", OwnerRef: owner.ID},
		&model.RuntimeCluster{ID: "clu_egress_project", Name: "Project", Scope: "project"},
		&model.ScopedResourceProjectBinding{ID: "srpb_egress_project", ResourceType: scopedResourceRuntimeCluster, ResourceID: "clu_egress_project", ProjectID: pay.ID},
		&model.DeploymentTarget{ID: "dplt_egress_pay", ProjectID: pay.ID, ApplicationID: "app_egress_pay", Name: "pay", Stage: "prod"},
		&model.DeploymentTarget{ID: "dplt_egress_other", ProjectID: other.ID, ApplicationID: "app_egress_other", Name: "other", Stage: "prod"},
	}
	for _, value := range seed {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}
	handlers := &Handlers{db: db, projects: repository.NewProjectRepository(db)}
	handlers.domains = newDomainHandlers(handlers)
	runtime := handlers.domains.runtime
	cluster := func(id string) gin.Params { return gin.Params{{Key: "clusterId", Value: id}} }
	body := func(cidr, name string, grants string) string {
		return `{"name":"` + name + `","cidr":"` + cidr + `","ports":"3306","protocol":"tcp","reason":"billing database","grants":` + grants + `}`
	}
	grantPay := `[{"projectId":"` + pay.ID + `","deploymentTargetIds":[],"includeBuilds":false}]`
	grantOther := `[{"projectId":"` + other.ID + `","deploymentTargetIds":[],"includeBuilds":false}]`

	t.Run("only cluster maintainers manage exceptions", func(t *testing.T) {
		recorder := egressExceptionRequest(t, runtime.CreateRuntimeClusterEgressException, stranger, http.MethodPost, cluster("clu_egress_global"), body("10.1.0.0/16", "db", grantPay))
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("stranger create = %d %s", recorder.Code, recorder.Body.String())
		}
		recorder = egressExceptionRequest(t, runtime.ListRuntimeClusterEgressExceptions, stranger, http.MethodGet, cluster("clu_egress_user"), "")
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("stranger list on personal cluster = %d", recorder.Code)
		}
	})

	t.Run("grants require projects that can deploy to the cluster", func(t *testing.T) {
		cases := []struct {
			name      string
			user      model.User
			clusterID string
			grants    string
			want      int
		}{
			{"personal cluster rejects projects the owner does not belong to", owner, "clu_egress_user", grantOther, http.StatusBadRequest},
			{"personal cluster accepts the owner's project", owner, "clu_egress_user", grantPay, http.StatusCreated},
			{"project cluster rejects unbound projects", admin, "clu_egress_project", grantOther, http.StatusBadRequest},
			{"project cluster accepts bound projects", admin, "clu_egress_project", grantPay, http.StatusCreated},
			{"unknown projects are indistinguishable from unusable ones", admin, "clu_egress_global", `[{"projectId":"prj_missing","deploymentTargetIds":[],"includeBuilds":false}]`, http.StatusBadRequest},
			{"targets must belong to the granted project", admin, "clu_egress_global", `[{"projectId":"` + pay.ID + `","deploymentTargetIds":["dplt_egress_other"],"includeBuilds":false}]`, http.StatusBadRequest},
		}
		for _, test := range cases {
			recorder := egressExceptionRequest(t, runtime.CreateRuntimeClusterEgressException, test.user, http.MethodPost, cluster(test.clusterID), body("10.1.0.0/16", "db", test.grants))
			if recorder.Code != test.want {
				t.Fatalf("%s: status = %d %s", test.name, recorder.Code, recorder.Body.String())
			}
			if test.want == http.StatusBadRequest && errorCode(t, recorder) != "runtime_cluster.egress_exception_grant_invalid" {
				t.Fatalf("%s: code = %s", test.name, errorCode(t, recorder))
			}
		}
	})

	t.Run("field validation uses field-specific codes and character counts", func(t *testing.T) {
		cases := []struct {
			body string
			code string
		}{
			{body("::ffff:0.0.0.0/96", "db", grantPay), "runtime_cluster.egress_exception_cidr_invalid"},
			{body("169.254.169.254/32", "db", grantPay), "runtime_cluster.egress_exception_cidr_invalid"},
			{body("10.1.0.0/16", strings.Repeat("库", 65), grantPay), "runtime_cluster.egress_exception_name_invalid"},
			{`{"name":"db","cidr":"10.1.0.0/16","ports":"0","protocol":"tcp","reason":"r","grants":[]}`, "runtime_cluster.egress_exception_ports_invalid"},
			{`{"name":"db","cidr":"10.1.0.0/16","ports":"","protocol":"icmp","reason":"r","grants":[]}`, "runtime_cluster.egress_exception_protocol_invalid"},
			{`{"name":"db","cidr":"10.1.0.0/16","ports":"","protocol":"tcp","reason":"","grants":[]}`, "runtime_cluster.egress_exception_reason_invalid"},
		}
		for _, test := range cases {
			recorder := egressExceptionRequest(t, runtime.CreateRuntimeClusterEgressException, admin, http.MethodPost, cluster("clu_egress_global"), test.body)
			if recorder.Code != http.StatusBadRequest || errorCode(t, recorder) != test.code {
				t.Fatalf("body %s: %d %s", test.body, recorder.Code, recorder.Body.String())
			}
		}
		recorder := egressExceptionRequest(t, runtime.CreateRuntimeClusterEgressException, admin, http.MethodPost, cluster("clu_egress_global"), body("10.1.0.0/16", strings.Repeat("库", 64), grantPay))
		if recorder.Code != http.StatusCreated {
			t.Fatalf("64 CJK characters must be accepted: %d %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("update replaces grants and delete cascades", func(t *testing.T) {
		recorder := egressExceptionRequest(t, runtime.CreateRuntimeClusterEgressException, admin, http.MethodPost, cluster("clu_egress_global"), body("10.2.0.0/16", "replace", grantPay))
		if recorder.Code != http.StatusCreated {
			t.Fatalf("create = %d %s", recorder.Code, recorder.Body.String())
		}
		var created struct {
			ID     string `json:"id"`
			Grants []struct {
				ProjectName string `json:"projectName"`
			} `json:"grants"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil || created.Grants[0].ProjectName != pay.Name {
			t.Fatalf("created = %s", recorder.Body.String())
		}
		params := gin.Params{{Key: "clusterId", Value: "clu_egress_global"}, {Key: "exceptionId", Value: created.ID}}
		recorder = egressExceptionRequest(t, runtime.UpdateRuntimeClusterEgressException, admin, http.MethodPut, params, body("10.2.0.0/16", "replace", grantOther))
		if recorder.Code != http.StatusOK {
			t.Fatalf("update = %d %s", recorder.Code, recorder.Body.String())
		}
		var grants []model.NetworkEgressExceptionGrant
		db.Where("exception_id = ?", created.ID).Find(&grants)
		if len(grants) != 1 || grants[0].ProjectID != other.ID {
			t.Fatalf("grants after update = %#v", grants)
		}
		recorder = egressExceptionRequest(t, runtime.DeleteRuntimeClusterEgressException, admin, http.MethodDelete, params, "")
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("delete = %d", recorder.Code)
		}
		var remaining int64
		db.Model(&model.NetworkEgressExceptionGrant{}).Where("exception_id = ?", created.ID).Count(&remaining)
		if remaining != 0 {
			t.Fatalf("grants must cascade on delete, remaining = %d", remaining)
		}
		recorder = egressExceptionRequest(t, runtime.UpdateRuntimeClusterEgressException, admin, http.MethodPut, params, body("10.2.0.0/16", "replace", grantPay))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("updating a deleted exception must not recreate it: %d", recorder.Code)
		}
		var resurrected int64
		db.Model(&model.NetworkEgressException{}).Where("id = ?", created.ID).Count(&resurrected)
		if resurrected != 0 {
			t.Fatal("deleted exception was recreated")
		}
	})

	t.Run("changes are audited without the CIDR", func(t *testing.T) {
		var logs []model.AuditLog
		db.Where("action like ?", "runtime_cluster.egress_exception.%").Find(&logs)
		if len(logs) == 0 {
			t.Fatal("egress exception changes must be audited")
		}
		for _, log := range logs {
			if strings.Contains(log.Message, "10.") {
				t.Fatalf("audit must not record CIDRs: %s", log.Message)
			}
		}
	})
}
