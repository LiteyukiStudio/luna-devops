package kubeproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/kubecatalog"
)

type gatewayTestAuthenticator struct {
	access AccessContext
	err    error
}

func (authenticator gatewayTestAuthenticator) Authenticate(context.Context, string, string) (AccessContext, error) {
	return authenticator.access, authenticator.err
}

func (authenticator gatewayTestAuthenticator) Revalidate(context.Context, AccessContext) (AccessContext, error) {
	return authenticator.access, authenticator.err
}

type gatewayTestUpstreams struct {
	upstream Upstream
	err      error
}

func (factory gatewayTestUpstreams) ForBinding(context.Context, AccessContext) (Upstream, error) {
	return factory.upstream, factory.err
}

type gatewayTestPreflight struct {
	check func(AccessContext, RequestInfo, *http.Request) error
}

func (preflight gatewayTestPreflight) Check(_ context.Context, access AccessContext, info RequestInfo, request *http.Request) error {
	if preflight.check == nil {
		return nil
	}
	return preflight.check(access, info, request)
}

type gatewayTestMutationPolicy struct{}

func (gatewayTestMutationPolicy) MutationContext(context.Context, AccessContext, RequestInfo) (MutationContext, error) {
	return MutationContext{}, nil
}

type gatewayTestRevalidationDenyAuthorizer struct {
	mu       sync.Mutex
	calls    int
	delegate Authorizer
}

func (authorizer *gatewayTestRevalidationDenyAuthorizer) Authorize(ctx context.Context, access AccessContext, info RequestInfo) (Decision, error) {
	authorizer.mu.Lock()
	authorizer.calls++
	call := authorizer.calls
	authorizer.mu.Unlock()
	if call > 1 {
		return Decision{Allowed: false}, nil
	}
	return authorizer.delegate.Authorize(ctx, access, info)
}

func (authorizer *gatewayTestRevalidationDenyAuthorizer) Calls() int {
	authorizer.mu.Lock()
	defer authorizer.mu.Unlock()
	return authorizer.calls
}

type gatewayTestContextBody struct {
	ctx context.Context
}

func (body gatewayTestContextBody) Read([]byte) (int, error) {
	<-body.ctx.Done()
	return 0, context.Cause(body.ctx)
}

func (gatewayTestContextBody) Close() error { return nil }

type gatewayTestDryRunner struct{}

func (gatewayTestDryRunner) Validate(_ context.Context, request *http.Request, _ Upstream, _ string, _ RequestInfo) (DryRunValidation, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return DryRunValidation{}, err
	}
	return DryRunValidation{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, CanonicalJSON: body, ClientBody: body}, nil
}

type gatewayTestAudit struct {
	mu      sync.Mutex
	begins  []AuditEvent
	results []AuditResult
	denials []AuditResult
}

func (audit *gatewayTestAudit) Begin(_ context.Context, event AuditEvent) (AuditAttempt, error) {
	audit.mu.Lock()
	defer audit.mu.Unlock()
	audit.begins = append(audit.begins, event)
	return AuditAttempt{ID: "attempt-1"}, nil
}

func (audit *gatewayTestAudit) Finish(_ context.Context, _ AuditAttempt, result AuditResult) error {
	audit.mu.Lock()
	defer audit.mu.Unlock()
	audit.results = append(audit.results, result)
	return nil
}

func (audit *gatewayTestAudit) RecordDenial(_ context.Context, _ AuditEvent, result AuditResult) error {
	audit.mu.Lock()
	defer audit.mu.Unlock()
	audit.denials = append(audit.denials, result)
	return nil
}

func gatewayForTest(upstream Upstream, recorder AuditRecorder) *Gateway {
	access := baseAccess()
	return &Gateway{
		Resolver:       NewRequestInfoResolver(),
		Authenticator:  gatewayTestAuthenticator{access: access},
		Authorizer:     CatalogAuthorizer{Catalog: kubecatalog.New()},
		Upstreams:      gatewayTestUpstreams{upstream: upstream},
		Preflight:      gatewayTestPreflight{},
		MutationPolicy: gatewayTestMutationPolicy{},
		Mutator:        NewMutator(),
		DryRunner:      gatewayTestDryRunner{},
		Proxy:          HTTPProxy{},
		Upgrade:        UpgradeProxy{},
		Metrics:        MetricsProxy{},
		Limiter:        NewLocalLimiter(DefaultLimiterConfig()),
		Streams:        DefaultStreamConfig(),
		Audit:          AuditCoordinator{Recorder: recorder},
		Telemetry:      NewTelemetry(slog.New(slog.NewTextHandler(io.Discard, nil))),
		ClientKey:      func(*http.Request) (ClientKey, error) { return ClientKey{Value: "127.0.0.1"}, nil },
	}
}

func TestGatewayCompositionProxiesDiscoveryWithoutForwardingCredential(t *testing.T) {
	baseURL, _ := url.Parse("https://upstream.internal")
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/version" {
			t.Fatalf("unexpected upstream path %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "" {
			t.Fatal("user credential reached upstream")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"gitVersion":"v1.36.2"}`))}, nil
	})
	gateway := gatewayForTest(Upstream{BaseURL: baseURL, Transport: transport}, nil)
	request := httptest.NewRequest(http.MethodGet, "https://gateway.example/kube/v1/bindings/b1/version", nil)
	request.Header.Set("Authorization", "Bearer credential-secret")
	escaped, err := ExtractEscapedKubePath(request, "b1")
	if err != nil {
		t.Fatal(err)
	}
	writer := httptest.NewRecorder()
	gateway.Handle(writer, request, "b1", escaped)
	if writer.Code != http.StatusOK || !strings.Contains(writer.Body.String(), "v1.36.2") {
		t.Fatalf("unexpected response: %d %s", writer.Code, writer.Body.String())
	}
	if gateway.Proxy.Telemetry != nil || gateway.Upgrade.Telemetry != nil || gateway.Metrics.Telemetry != nil {
		t.Fatal("request handling mutated the shared Gateway configuration")
	}
}

func TestGatewayCompositionMutatesDryRunsAuditsThenExecutes(t *testing.T) {
	baseURL, _ := url.Parse("https://upstream.internal")
	var upstreamBody []byte
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var err error
		upstreamBody, err = io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusCreated, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"apiVersion":"v1","kind":"Pod","metadata":{"name":"app"}}`))}, nil
	})
	audit := &gatewayTestAudit{}
	gateway := gatewayForTest(Upstream{BaseURL: baseURL, Transport: transport}, audit)
	body := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"app"},"spec":{"containers":[{"name":"app","image":"example/app","securityContext":{"allowPrivilegeEscalation":false}}]}}`
	request := httptest.NewRequest(http.MethodPost, "https://gateway.example/kube/v1/bindings/b1/api/v1/namespaces/project-a/pods", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer credential-secret")
	request.Header.Set("Content-Type", "application/json")
	escaped, err := ExtractEscapedKubePath(request, "b1")
	if err != nil {
		t.Fatal(err)
	}
	writer := httptest.NewRecorder()
	gateway.Handle(writer, request, "b1", escaped)
	if writer.Code != http.StatusCreated {
		t.Fatalf("unexpected response: %d %s", writer.Code, writer.Body.String())
	}
	var object map[string]any
	if err := json.Unmarshal(upstreamBody, &object); err != nil {
		t.Fatal(err)
	}
	metadata := object["metadata"].(map[string]any)
	labels := metadata["labels"].(map[string]any)
	if labels["luna.devops/project-id"] != "p1" || labels["luna.devops/management-source"] != "kubectl" || metadata["namespace"] != "project-a" {
		t.Fatalf("upstream object lacks ownership: %#v", metadata)
	}
	if len(audit.begins) != 1 || len(audit.results) != 1 || !audit.results[0].Allowed || audit.results[0].StatusCode != http.StatusCreated {
		t.Fatalf("unexpected audit lifecycle: begins=%#v results=%#v", audit.begins, audit.results)
	}
}

func TestGatewayAuthorizationDenialIsAuditedBeforeAnyUpstreamCall(t *testing.T) {
	baseURL, _ := url.Parse("https://upstream.internal")
	upstreamCalled := false
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		upstreamCalled = true
		return nil, nil
	})
	audit := &gatewayTestAudit{}
	gateway := gatewayForTest(Upstream{BaseURL: baseURL, Transport: transport}, audit)
	access := baseAccess()
	access.ProjectRole = authz.ProjectRoleViewer
	gateway.Authenticator = gatewayTestAuthenticator{access: access}
	request := httptest.NewRequest(http.MethodPost, "https://gateway.example/kube/v1/bindings/b1/apis/apps/v1/namespaces/project-a/deployments", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer credential-secret")
	request.Header.Set("Content-Type", "application/json")
	escaped, err := ExtractEscapedKubePath(request, "b1")
	if err != nil {
		t.Fatal(err)
	}
	writer := httptest.NewRecorder()
	gateway.Handle(writer, request, "b1", escaped)
	if writer.Code != http.StatusForbidden || upstreamCalled {
		t.Fatalf("denial escaped policy: status=%d upstream=%v", writer.Code, upstreamCalled)
	}
	if len(audit.denials) != 1 || audit.denials[0].Allowed {
		t.Fatalf("denial was not persistently audited: %#v", audit.denials)
	}
}

func TestGatewayWatchStopsWhenRevalidationReturnsDenyWithoutError(t *testing.T) {
	baseURL, _ := url.Parse("https://upstream.internal")
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       gatewayTestContextBody{ctx: request.Context()},
		}, nil
	})
	audit := &gatewayTestAudit{}
	gateway := gatewayForTest(Upstream{BaseURL: baseURL, Transport: transport}, audit)
	authorizer := &gatewayTestRevalidationDenyAuthorizer{delegate: CatalogAuthorizer{Catalog: kubecatalog.New()}}
	gateway.Authorizer = authorizer
	gateway.Streams = StreamConfig{RevalidateInterval: time.Millisecond, WatchMaxDuration: time.Minute, IdleTimeout: time.Minute}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "https://gateway.example/kube/v1/bindings/b1/api/v1/namespaces/project-a/pods?watch=true", nil).WithContext(ctx)
	request.Header.Set("Authorization", "Bearer credential-secret")
	escaped, err := ExtractEscapedKubePath(request, "b1")
	if err != nil {
		t.Fatal(err)
	}
	writer := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		gateway.Handle(writer, request, "b1", escaped)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("watch remained open after the authorizer returned deny without an error")
	}
	if authorizer.Calls() < 2 {
		t.Fatalf("stream authorization was not revalidated: calls=%d", authorizer.Calls())
	}
	if len(audit.results) != 1 || audit.results[0].StreamTerminal != "authorization_revoked" {
		t.Fatalf("unexpected stream audit result: %#v", audit.results)
	}
}

func TestExtractEscapedKubePathPreservesUnsafeEncodingForResolver(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://gateway.example/kube/v1/bindings/b1/api/v1/namespaces/project-a%252Fforeign/pods", nil)
	path, err := ExtractEscapedKubePath(request, "b1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(path), "%252f") {
		t.Fatalf("escaped path was decoded prematurely: %q", path)
	}
	if _, err := NewRequestInfoResolver().Resolve(request, path); err == nil {
		t.Fatal("recursively escaped slash must be rejected")
	}
}

func TestGatewayBoundsNonUpgradeSubresourceAndDeleteBodies(t *testing.T) {
	baseURL, _ := url.Parse("https://upstream.internal")
	upstreamCalled := false
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		upstreamCalled = true
		return nil, nil
	})
	payload := bytes.Repeat([]byte{'x'}, int(DefaultMaxRequestBodyBytes)+1)
	for _, test := range []struct {
		name   string
		method string
		path   string
	}{
		{name: "scale", method: http.MethodPatch, path: "/apis/apps/v1/namespaces/project-a/deployments/app/scale"},
		{name: "eviction", method: http.MethodPost, path: "/api/v1/namespaces/project-a/pods/app/eviction"},
		{name: "delete-options", method: http.MethodDelete, path: "/api/v1/namespaces/project-a/pods/app"},
	} {
		t.Run(test.name, func(t *testing.T) {
			upstreamCalled = false
			audit := &gatewayTestAudit{}
			gateway := gatewayForTest(Upstream{BaseURL: baseURL, Transport: transport}, audit)
			request := httptest.NewRequest(test.method, "https://gateway.example/kube/v1/bindings/b1"+test.path, bytes.NewReader(payload))
			request.Header.Set("Authorization", "Bearer credential-secret")
			request.Header.Set("Content-Type", "application/json")
			escaped, err := ExtractEscapedKubePath(request, "b1")
			if err != nil {
				t.Fatal(err)
			}
			writer := httptest.NewRecorder()
			gateway.Handle(writer, request, "b1", escaped)
			if writer.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("oversized %s body returned %d: %s", test.name, writer.Code, writer.Body.String())
			}
			if upstreamCalled {
				t.Fatalf("oversized %s body reached upstream", test.name)
			}
			if len(audit.denials) != 1 || audit.denials[0].ErrorCode != CodeRequestTooLarge {
				t.Fatalf("oversized %s denial was not audited: %#v", test.name, audit.denials)
			}
		})
	}
}
