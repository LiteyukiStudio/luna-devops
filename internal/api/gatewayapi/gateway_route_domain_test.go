package gatewayapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type gatewayDomainTestHost struct {
	Host
	db             *gorm.DB
	cluster        model.RuntimeCluster
	clusterLoadErr error
}

func (host gatewayDomainTestHost) DBFor(ctx *gin.Context) *gorm.DB {
	return host.db.WithContext(ctx.Request.Context())
}

func (host gatewayDomainTestHost) DBWithContext(ctx context.Context) *gorm.DB {
	return host.db.WithContext(ctx)
}

func (gatewayDomainTestHost) SecretStore() secret.Store { return secret.Store{} }

func (host gatewayDomainTestHost) RuntimeClusterForDeploymentTarget(context.Context, model.DeploymentTarget) (model.RuntimeCluster, error) {
	return host.cluster, host.clusterLoadErr
}

func TestGatewayClusterForDomainCheckRequiresRuntimeReference(t *testing.T) {
	handler := New(gatewayDomainTestHost{})
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/", nil)

	_, err := handler.gatewayClusterForDomainCheck(ctx)
	if !errors.Is(err, errGatewayDomainRuntimeReferenceRequired) {
		t.Fatalf("error = %v, want runtime reference required", err)
	}
}

func TestGatewayClusterForDomainCheckPropagatesMissingReferences(t *testing.T) {
	db := openGatewayDomainTestDB(t)
	handler := New(gatewayDomainTestHost{db: db})

	for _, test := range []struct {
		name  string
		query string
	}{
		{name: "route", query: "routeId=gwr_missing"},
		{name: "deployment target", query: "deploymentTargetId=dplt_missing"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("GET", "/?"+test.query, nil)
			ctx.Params = gin.Params{{Key: "projectId", Value: "proj_1"}}

			cluster, err := handler.gatewayClusterForDomainCheck(ctx)
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("error = %v, want gorm.ErrRecordNotFound", err)
			}
			if cluster.ID != "" {
				t.Fatalf("cluster = %#v, want empty", cluster)
			}
		})
	}
}

func TestGatewayRouteRuntimeClusterObservationCodePreservesUnavailableSemantics(t *testing.T) {
	if got := gatewayRouteRuntimeClusterObservationCode(gorm.ErrRecordNotFound); got != "gateway_route.runtime_cluster_not_configured" {
		t.Fatalf("missing reference code = %q", got)
	}
	if got := gatewayRouteRuntimeClusterObservationCode(errors.New("database unavailable")); got != "gateway_route.runtime_cluster_unavailable" {
		t.Fatalf("database failure code = %q", got)
	}
}

func TestGatewayRuntimeClusterErrorUsesStableHTTPResponses(t *testing.T) {
	for _, test := range []struct {
		name       string
		err        error
		statusCode int
		code       string
	}{
		{name: "reference required", err: errGatewayDomainRuntimeReferenceRequired, statusCode: http.StatusBadRequest, code: "gateway_route.runtime_cluster_reference_required"},
		{name: "reference missing", err: gorm.ErrRecordNotFound, statusCode: http.StatusNotFound, code: "gateway_route.runtime_cluster_missing"},
		{name: "storage unavailable", err: errors.New("database unavailable"), statusCode: http.StatusServiceUnavailable, code: "gateway_route.runtime_cluster_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			New(gatewayDomainTestHost{}).writeGatewayRuntimeClusterError(ctx, test.err)

			if recorder.Code != test.statusCode || !strings.Contains(recorder.Body.String(), `"code":"`+test.code+`"`) {
				t.Fatalf("response = %d %s, want %d/%s", recorder.Code, recorder.Body.String(), test.statusCode, test.code)
			}
		})
	}
}

func TestGatewayRouteAccessURLPropagatesRuntimeClusterFailure(t *testing.T) {
	db := openGatewayDomainTestDB(t)
	if err := db.Exec(`INSERT INTO deployment_targets (id, project_id, cluster_id) VALUES (?, ?, ?)`, "dplt_1", "proj_1", "cluster_1").Error; err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("runtime cluster storage unavailable")
	handler := New(gatewayDomainTestHost{db: db, clusterLoadErr: wantErr})
	route := model.GatewayRoute{ProjectID: "proj_1", DeploymentTargetID: "dplt_1", Host: "app.example.com", Path: "/"}

	resolved, err := handler.gatewayRouteWithAccessURL(route, context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if resolved.AccessURL != "" {
		t.Fatalf("access URL = %q, want empty", resolved.AccessURL)
	}
}

func TestGatewayRouteListMarksMissingRuntimeReferenceUnavailable(t *testing.T) {
	db := openGatewayDomainTestDB(t)
	handler := New(gatewayDomainTestHost{db: db})
	route := model.GatewayRoute{ProjectID: "proj_1", DeploymentTargetID: "dplt_missing", Host: "app.example.com", Path: "/"}

	resolved := handler.gatewayRoutesWithAccessURL([]model.GatewayRoute{route}, context.Background())
	if len(resolved) != 1 {
		t.Fatalf("route count = %d, want 1", len(resolved))
	}
	if resolved[0].AccessURL != "" {
		t.Fatalf("access URL = %q, want empty", resolved[0].AccessURL)
	}
	if resolved[0].Status != observation.StatusUnavailable || resolved[0].ObservationCode != "gateway_route.runtime_cluster_not_configured" {
		t.Fatalf("observation = %q/%q, want unavailable/runtime_cluster_not_configured", resolved[0].Status, resolved[0].ObservationCode)
	}
}

func openGatewayDomainTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testdb.Open(t, testdb.Options{
		SchemaPrefix: "gateway_domain",
		Migrate: func(db *gorm.DB) error {
			if err := db.Exec(`CREATE TABLE deployment_targets (
				id text PRIMARY KEY,
				project_id text NOT NULL,
				cluster_id text NOT NULL DEFAULT '',
				deleted_at timestamptz
			)`).Error; err != nil {
				return err
			}
			return db.Exec(`CREATE TABLE gateway_routes (
				id text PRIMARY KEY,
				project_id text NOT NULL,
				deployment_target_id text NOT NULL DEFAULT '',
				deleted_at timestamptz
			)`).Error
		},
	})
}
