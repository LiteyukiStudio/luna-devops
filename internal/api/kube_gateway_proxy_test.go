package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/kubeaccess"
	"github.com/LiteyukiStudio/devops/internal/kubepolicy"
	"github.com/LiteyukiStudio/devops/internal/kubeproxy"
	"github.com/LiteyukiStudio/devops/internal/model"
	apiTelemetry "github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestKubeGatewayRoutesUseExplicitMethodsAndKubernetesStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.Use(cors(Config{}))
	registerKubeGatewayRoutes(router, &Handlers{})
	router.NoMethod(kubeGatewayNoMethod)

	wanted := map[string]bool{}
	for _, method := range kubeGatewayHTTPMethods {
		wanted[method] = true
	}
	for _, route := range router.Routes() {
		if strings.HasPrefix(route.Path, "/kube/v1/bindings/") && !wanted[route.Method] {
			t.Fatalf("unexpected Kubernetes protocol method %q", route.Method)
		}
	}

	for _, method := range []string{http.MethodOptions, http.MethodConnect, http.MethodTrace} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, "/kube/v1/bindings/kbd_test/api/v1/namespaces", nil)
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d body=%s", method, recorder.Code, recorder.Body.String())
		}
		if !strings.Contains(recorder.Header().Get("Allow"), http.MethodPatch) {
			t.Fatalf("%s Allow = %q", method, recorder.Header().Get("Allow"))
		}
		var status metav1.Status
		if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil || status.Kind != "Status" || status.Reason != metav1.StatusReasonMethodNotAllowed {
			t.Fatalf("%s did not return Kubernetes Status: status=%#v err=%v", method, status, err)
		}
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/kube/v1/bindings/kbd_test/version", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), kubeproxy.CodeUnavailable) {
		t.Fatalf("configured GET status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestKubeGatewayPortForwardPortsAreBoundToPodAndMatchingServices(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api"}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: 8080}}}}},
	}
	services := []corev1.Service{
		{Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "api"}, Ports: []corev1.ServicePort{
			{Port: 80, TargetPort: intstr.FromString("http")},
			{Port: 90, TargetPort: intstr.FromInt32(9090)},
		}}},
		{Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "other"}, Ports: []corev1.ServicePort{
			{Port: 70, TargetPort: intstr.FromInt32(7070)},
		}}},
	}
	allowed := kubeGatewayPortForwardPorts(pod, services)
	for _, port := range []int32{8080, 9090} {
		if _, ok := allowed[port]; !ok {
			t.Errorf("expected port %d to be allowed", port)
		}
	}
	if _, ok := allowed[7070]; ok {
		t.Fatal("unmatched Service port was allowed")
	}
}

func TestKubeGatewayAuthenticationProjectionContainsOnlyAuthorizationState(t *testing.T) {
	applicationID := "app_test"
	expiresAt := time.Now().UTC().Add(time.Hour)
	access := kubeGatewayAccessContext(kubeaccess.Authentication{
		Token:   model.AccessToken{ID: "tok_test", Scope: "kube:read,kube:connect", ExpiresAt: &expiresAt},
		User:    model.User{ID: "usr_test", Role: authz.PlatformRoleUser},
		Binding: model.KubeAccessBinding{ID: "kbd_test", ApplicationID: &applicationID},
		Project: model.Project{ID: "prj_test", KubernetesNamespace: "luna-prj-test"},
		Cluster: model.RuntimeCluster{ID: "rcl_test"},
		Access:  authz.ProjectAccess{Role: authz.ProjectRoleDeveloper},
	})
	if access.UserID != "usr_test" || access.BindingID != "kbd_test" || access.ApplicationID != applicationID || access.Namespace != "luna-prj-test" || !access.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("access projection = %#v", access)
	}
}

func TestKubeGatewayPersistentAuditStartsAndDenialsAsFailure(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "kube_gateway_audit_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.AuditLog{})
		},
	})
	recorder := kubeGatewayAuditRecorder{handlers: &Handlers{db: db}}
	event := kubeproxy.AuditEvent{
		ActorID: "usr_audit", BindingID: "kbd_audit", ProjectID: "prj_audit",
		Verb: "create", Resource: "pods", Namespace: "luna-prj-audit",
	}
	attempt, err := recorder.Begin(t.Context(), event)
	if err != nil {
		t.Fatalf("begin audit: %v", err)
	}
	if err := recorder.RecordDenial(t.Context(), event, kubeproxy.AuditResult{
		Allowed: false, StatusCode: http.StatusForbidden, Outcome: "denied", ErrorCode: kubeproxy.CodeForbidden,
	}); err != nil {
		t.Fatalf("record denial: %v", err)
	}
	var audits []model.AuditLog
	if err := db.Find(&audits).Error; err != nil {
		t.Fatalf("load audits: %v", err)
	}
	failures := 0
	foundAttempt := false
	for _, audit := range audits {
		if !audit.Success {
			failures++
		}
		foundAttempt = foundAttempt || audit.ID == attempt.ID
	}
	if len(audits) != 2 || failures != 2 || !foundAttempt {
		t.Fatalf("persistent audits = %#v", audits)
	}
}

func TestKubeGatewayClientKeyDoesNotTrustForwardedHeaderDirectly(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://gateway.example/kube/v1/bindings/kbd_test/version", nil)
	request.RemoteAddr = "192.0.2.10:8443"
	request.Header.Set("X-Forwarded-For", "198.51.100.7")
	remote, err := kubeGatewayClientKey(request)
	if err != nil {
		t.Fatal(err)
	}
	trustedRequest := request.WithContext(context.WithValue(request.Context(), kubeGatewayClientKeyContextKey{}, "198.51.100.7"))
	trusted, err := kubeGatewayClientKey(trustedRequest)
	if err != nil {
		t.Fatal(err)
	}
	if remote.Value == "" || trusted.Value == "" || remote.Value == trusted.Value || strings.Contains(remote.Value, "192.0.2.10") {
		t.Fatalf("client keys were not isolated: remote=%q trusted=%q", remote.Value, trusted.Value)
	}
}

func TestKubeGatewayOwnershipCheckFailsClosed(t *testing.T) {
	access := kubeproxy.AccessContext{ProjectID: "prj_test", ApplicationID: "app_test", Namespace: "luna-prj-test"}
	metadata := &metav1.ObjectMeta{Namespace: access.Namespace, Labels: map[string]string{
		kubepolicy.ManagedByLabel:        kubepolicy.ManagedByValue,
		kubepolicy.ProjectIDLabel:        access.ProjectID,
		kubepolicy.ApplicationIDLabel:    access.ApplicationID,
		kubepolicy.ManagementSourceLabel: string(kubepolicy.ManagementSourceKubectl),
	}}
	if !kubeGatewayMetadataOwnedByAccess(metadata, access) {
		t.Fatal("valid ownership metadata was rejected")
	}
	metadata.Labels[kubepolicy.ProjectIDLabel] = "prj_other"
	if kubeGatewayMetadataOwnedByAccess(metadata, access) {
		t.Fatal("cross-project metadata was accepted")
	}
}

func TestKubeGatewayRoutesRequireBearerOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handlers := &Handlers{}
	handlers.kubeGateway = newKubeGateway(handlers)
	router := gin.New()
	registerKubeGatewayRoutes(router, handlers)

	for _, test := range []struct {
		name          string
		authorization []string
		cookie        string
		wantStatus    int
	}{
		{name: "cookie only", cookie: sessionCookieName + "=session-secret", wantStatus: http.StatusUnauthorized},
		{name: "basic auth", authorization: []string{"Basic dXNlcjpwYXNz"}, wantStatus: http.StatusUnauthorized},
		{name: "multiple bearer headers", authorization: []string{"Bearer token-one", "Bearer token-two"}, wantStatus: http.StatusUnauthorized},
		{name: "single bearer reaches authenticator", authorization: []string{"Bearer kube-secret"}, wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/kube/v1/bindings/kbd_test/version", nil)
			for _, value := range test.authorization {
				request.Header.Add("Authorization", value)
			}
			if test.cookie != "" {
				request.Header.Set("Cookie", test.cookie)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
			var status metav1.Status
			if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil || status.Kind != "Status" {
				t.Fatalf("response was not Kubernetes Status: %#v err=%v", status, err)
			}
			if test.wantStatus == http.StatusUnauthorized && recorder.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatalf("WWW-Authenticate = %q", recorder.Header().Get("WWW-Authenticate"))
			}
		})
	}
}

func TestKubeGatewayRoutesPrecedePlatformBusinessMiddleware(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test password=test dbname=test port=1 sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(db, mustTestConfig(t))
	for _, test := range []struct {
		path       string
		wantStatus int
	}{
		{path: "/kube/v1/bindings/kbd_test/version", wantStatus: http.StatusUnauthorized},
		{path: "/kube/unknown", wantStatus: http.StatusNotFound},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.Header.Set(aiRunIDHeader, "unrelated-run")
		request.Header.Set(aiToolCallIDHeader, "unrelated-tool-call")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != test.wantStatus {
			t.Fatalf("GET %s status = %d body=%s", test.path, response.Code, response.Body.String())
		}
		var status metav1.Status
		if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil || status.Kind != "Status" || int(status.Code) != test.wantStatus {
			t.Fatalf("GET %s crossed into platform JSON middleware: status=%#v err=%v body=%s", test.path, status, err, response.Body.String())
		}
	}
}

func TestKubeGatewayRouteOwnsSingleRedactedServerSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	})

	gin.SetMode(gin.TestMode)
	handlers := &Handlers{}
	handlers.kubeGateway = newKubeGateway(handlers)
	router := gin.New()
	router.Use(apiTelemetry.GinTracingMiddleware("test-api"))
	registerKubeGatewayRoutes(router, handlers)

	request := httptest.NewRequest(http.MethodGet, "/kube/v1/bindings/sensitive-binding/version?token=sensitive-query", nil)
	request.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}

	wantParent, err := trace.SpanIDFromHex("0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	serverSpans := 0
	for _, span := range recorder.Ended() {
		if span.SpanKind() != trace.SpanKindServer {
			continue
		}
		serverSpans++
		if span.Name() != "kube.gateway.request" || span.Parent().SpanID() != wantParent || span.Status().Code != codes.Error {
			t.Fatalf("server span = name %q parent %s status %v", span.Name(), span.Parent().SpanID(), span.Status())
		}
		text := span.Name()
		for _, value := range span.Attributes() {
			text += string(value.Key) + "=" + value.Value.Emit()
		}
		for _, sensitive := range []string{"sensitive-binding", "sensitive-query", "/kube/", "token="} {
			if strings.Contains(text, sensitive) {
				t.Fatalf("server span leaked %q: %s", sensitive, text)
			}
		}
	}
	if serverSpans != 1 {
		t.Fatalf("server span count = %d, want exactly one", serverSpans)
	}
}

func TestKubeGatewayRouterFailuresOwnRedactedServerSpans(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	})

	gin.SetMode(gin.TestMode)
	handlers := &Handlers{}
	handlers.kubeGateway = newKubeGateway(handlers)
	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.Use(apiTelemetry.GinTracingMiddleware("test-api"))
	registerKubeGatewayRoutes(router, handlers)
	router.NoMethod(kubeGatewayNoMethodHandler(handlers))
	registerStaticUI(router, nil, nil, handlers.HandleKubeGatewayNoRoute)

	for _, test := range []struct {
		method string
		path   string
		status int
	}{
		{method: http.MethodOptions, path: "/kube/v1/bindings/sensitive-binding/version", status: http.StatusMethodNotAllowed},
		{method: http.MethodGet, path: "/kube/sensitive-unknown", status: http.StatusNotFound},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
		if response.Code != test.status {
			t.Fatalf("%s %s status = %d body=%s", test.method, test.path, response.Code, response.Body.String())
		}
	}

	serverSpans := 0
	for _, span := range recorder.Ended() {
		if span.SpanKind() != trace.SpanKindServer {
			continue
		}
		serverSpans++
		if span.Name() != "kube.gateway.request" || span.Status().Code != codes.Error {
			t.Fatalf("unexpected fallback span: name=%q status=%v", span.Name(), span.Status())
		}
		text := span.Name()
		for _, value := range span.Attributes() {
			text += string(value.Key) + "=" + value.Value.Emit()
		}
		for _, sensitive := range []string{"sensitive-binding", "sensitive-unknown", "/kube/"} {
			if strings.Contains(text, sensitive) {
				t.Fatalf("fallback span leaked %q: %s", sensitive, text)
			}
		}
	}
	if serverSpans != 2 {
		t.Fatalf("fallback server span count = %d, want two", serverSpans)
	}
}

func TestKubeGatewayApplyMayCreateOnlyForRootServerSideApplyPatch(t *testing.T) {
	base := kubeproxy.RequestInfo{Verb: "patch", Name: "api", IsApplyPatch: true}
	if !kubeGatewayApplyMayCreate(base) {
		t.Fatal("root server-side apply PATCH was not allowed to create")
	}
	for _, info := range []kubeproxy.RequestInfo{
		{Verb: "patch", Name: "api"},
		{Verb: "patch", IsApplyPatch: true},
		{Verb: "patch", Name: "api", Subresource: "status", IsApplyPatch: true},
		{Verb: "update", Name: "api", IsApplyPatch: true},
	} {
		if kubeGatewayApplyMayCreate(info) {
			t.Fatalf("non-create apply shape was allowed: %#v", info)
		}
	}
}

func TestKubeGatewayAllowedGatewayParentsUseExactConfiguredIdentity(t *testing.T) {
	for _, test := range []struct {
		name     string
		cluster  model.RuntimeCluster
		want     string
		rejected []string
	}{
		{
			name: "defaults", want: kubepolicy.GatewayParentKey("kube-system", "luna-gateway"),
			rejected: []string{kubepolicy.GatewayParentKey("other-system", "luna-gateway"), kubepolicy.GatewayParentKey("kube-system", "other-gateway")},
		},
		{
			name: "cluster configuration", cluster: model.RuntimeCluster{GatewayNamespace: "edge-system", GatewayName: "shared"},
			want:     kubepolicy.GatewayParentKey("edge-system", "shared"),
			rejected: []string{kubepolicy.GatewayParentKey("project-a", "shared"), kubepolicy.GatewayParentKey("edge-system", "luna-gateway")},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			allowed := kubeGatewayAllowedGatewayParents(test.cluster)
			if len(allowed) != 1 {
				t.Fatalf("allowed parents = %#v", allowed)
			}
			if _, ok := allowed[test.want]; !ok {
				t.Fatalf("configured parent %q was not allowed: %#v", test.want, allowed)
			}
			for _, value := range test.rejected {
				if _, ok := allowed[value]; ok {
					t.Fatalf("unconfigured parent %q was allowed", value)
				}
			}
		})
	}
}
