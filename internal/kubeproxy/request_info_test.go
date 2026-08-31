package kubeproxy

import (
	"net/http"
	"testing"
)

func request(t *testing.T, method, target string) *http.Request {
	t.Helper()
	value, err := http.NewRequest(method, "https://gateway.example"+target, nil)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestRequestInfoParsesNamespacedWatch(t *testing.T) {
	value := request(t, http.MethodGet, "/apis/apps/v1/namespaces/project-a/deployments?watch=true")
	info, err := NewRequestInfoResolver().Resolve(value, value.URL.EscapedPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Verb != "watch" || info.Namespace != "project-a" || info.Resource != "deployments" || info.Transport != TransportWatch {
		t.Fatalf("unexpected info: %#v", info)
	}
}

func TestRequestInfoRejectsEncodedTraversalAndSlash(t *testing.T) {
	for _, path := range []string{"/api/v1/namespaces/%2e%2e/pods", "/api/v1/namespaces/project-a%2Fother/pods", "/api//v1/pods"} {
		value := request(t, http.MethodGet, "/version")
		if _, err := NewRequestInfoResolver().Resolve(value, path); err == nil {
			t.Fatalf("expected %q to be rejected", path)
		}
	}
}

func TestRequestInfoParsesWebSocketConnect(t *testing.T) {
	value := request(t, http.MethodGet, "/api/v1/namespaces/project-a/pods/app/exec")
	value.Header.Set("Connection", "Upgrade")
	value.Header.Set("Upgrade", "websocket")
	info, err := NewRequestInfoResolver().Resolve(value, value.URL.EscapedPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Verb != "connect" || !info.IsUpgrade || info.Transport != TransportUpgrade {
		t.Fatalf("unexpected info: %#v", info)
	}
}

func TestRequestInfoRejectsUnknownUpgrade(t *testing.T) {
	value := request(t, http.MethodGet, "/api/v1/namespaces/project-a/pods/app/exec")
	value.Header.Set("Connection", "Upgrade")
	value.Header.Set("Upgrade", "h2c")
	if _, err := NewRequestInfoResolver().Resolve(value, value.URL.EscapedPath()); err == nil {
		t.Fatal("unknown upgrade must be rejected")
	}
}

func TestRequestInfoRejectsAmbiguousBooleanQueryAndNonConnectUpgrade(t *testing.T) {
	value := request(t, http.MethodGet, "/api/v1/namespaces/project-a/pods?watch=false&watch=true")
	if _, err := NewRequestInfoResolver().Resolve(value, value.URL.EscapedPath()); err == nil {
		t.Fatal("repeated watch query must be rejected")
	}
	value = request(t, http.MethodGet, "/api/v1/namespaces/project-a/pods/app")
	value.Header.Set("Connection", "Upgrade")
	value.Header.Set("Upgrade", "websocket")
	if _, err := NewRequestInfoResolver().Resolve(value, value.URL.EscapedPath()); err == nil {
		t.Fatal("Upgrade outside a connect subresource must be rejected")
	}
}

func TestRequestInfoRequiresProtocolSpecificConnectMethod(t *testing.T) {
	value := request(t, http.MethodPost, "/api/v1/namespaces/project-a/pods/app/exec")
	value.Header.Set("Connection", "Upgrade")
	value.Header.Set("Upgrade", "websocket")
	if _, err := NewRequestInfoResolver().Resolve(value, value.URL.EscapedPath()); err == nil {
		t.Fatal("WebSocket connect over POST must be rejected")
	}
	value = request(t, http.MethodGet, "/api/v1/namespaces/project-a/pods/app/exec")
	value.Header.Set("Connection", "Upgrade")
	value.Header.Set("Upgrade", "SPDY/3.1")
	if _, err := NewRequestInfoResolver().Resolve(value, value.URL.EscapedPath()); err == nil {
		t.Fatal("SPDY connect over GET must be rejected")
	}
}

func TestNamespaceFinalizeIsParsedAsProtectedSubresource(t *testing.T) {
	value := request(t, http.MethodPut, "/api/v1/namespaces/project-a/finalize")
	info, err := NewRequestInfoResolver().Resolve(value, value.URL.EscapedPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Resource != "namespaces" || info.Name != "project-a" || info.Subresource != "finalize" || info.Namespace != "" {
		t.Fatalf("unexpected namespace finalize RequestInfo: %#v", info)
	}
}

func TestRequestedPortForwardPortsAreBoundedAndCanonical(t *testing.T) {
	value := request(t, http.MethodGet, "/api/v1/namespaces/project-a/pods/app/portforward?ports=8080,443&ports=8080")
	ports, err := RequestedPortForwardPorts(value)
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 2 || ports[0] != 443 || ports[1] != 8080 {
		t.Fatalf("unexpected ports: %#v", ports)
	}
	value = request(t, http.MethodGet, "/api/v1/namespaces/project-a/pods/app/portforward?ports=0")
	if _, err := RequestedPortForwardPorts(value); err == nil {
		t.Fatal("invalid port must be rejected")
	}
}

func TestRequestInfoRecognizesOnlyServerSideApplyPatch(t *testing.T) {
	for _, contentType := range []string{"application/apply-patch+yaml", "application/apply-patch+json; charset=utf-8"} {
		value := request(t, http.MethodPatch, "/apis/apps/v1/namespaces/project-a/deployments/app")
		value.Header.Set("Content-Type", contentType)
		info, err := NewRequestInfoResolver().Resolve(value, value.URL.EscapedPath())
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsApplyPatch {
			t.Fatalf("apply Content-Type %q was not recognized", contentType)
		}
	}
	for _, contentType := range []string{"application/merge-patch+json", "application/strategic-merge-patch+json", "application/json-patch+json"} {
		value := request(t, http.MethodPatch, "/apis/apps/v1/namespaces/project-a/deployments/app")
		value.Header.Set("Content-Type", contentType)
		info, err := NewRequestInfoResolver().Resolve(value, value.URL.EscapedPath())
		if err != nil {
			t.Fatal(err)
		}
		if info.IsApplyPatch {
			t.Fatalf("non-apply Content-Type %q was recognized as apply", contentType)
		}
	}
}
