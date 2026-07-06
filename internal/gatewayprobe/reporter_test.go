package gatewayprobe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAPIReporterSendsHello(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/api/v1/billing/gateway-traffic/hello" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	reporter := NewAPIReporter(server.URL, "token", time.Second)
	if err := reporter.Hello(context.Background()); err != nil {
		t.Fatalf("Hello returned error: %v", err)
	}
}

func TestAPIReporterSendsGatewayTrafficPayload(t *testing.T) {
	var got gatewayTrafficPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/api/v1/billing/gateway-traffic" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	reporter := NewAPIReporter(server.URL, "token", time.Second)
	err := reporter.Report(context.Background(), RouteUsageWindow{
		RouteID:       "gwr_1",
		ResponseBytes: 4096,
		RequestCount:  12,
		PeriodStart:   time.Date(2026, 7, 6, 1, 2, 0, 0, time.UTC),
		PeriodEnd:     time.Date(2026, 7, 6, 1, 3, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if got.RouteID != "gwr_1" || got.ResponseBytes != 4096 || got.RequestCount != 12 {
		t.Fatalf("payload = %#v", got)
	}
	if got.PeriodStart != "2026-07-06T01:02:00Z" || got.PeriodEnd != "2026-07-06T01:03:00Z" {
		t.Fatalf("period = %#v", got)
	}
}
