package agentobservability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPrometheusPointRejectsNonFiniteValues(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-Inf"} {
		raw := []json.RawMessage{json.RawMessage(`1720000000`), json.RawMessage(`"` + value + `"`)}
		if _, ok := prometheusPoint(raw); ok {
			t.Fatalf("non-finite Prometheus value %q was accepted", value)
		}
	}
}

func TestPrometheusClientQueriesAndPropagatesAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer secret" || request.Header.Get("X-Scope-OrgID") != "tenant-a" {
			t.Fatalf("missing query authentication headers")
		}
		if request.URL.Path != "/api/v1/query" || request.URL.Query().Get("query") == "" {
			t.Fatalf("unexpected request: %s", request.URL.String())
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1720000000,"2.5"]}]}}`)
	}))
	defer server.Close()
	client, err := New(SourcePrometheus, Config{BaseURL: server.URL, Token: "secret", TenantID: "tenant-a"})
	if err != nil {
		t.Fatal(err)
	}
	series, err := client.Query(context.Background(), "vector(1)", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 || len(series[0].Points) != 1 || series[0].Points[0].Value != 2.5 {
		t.Fatalf("unexpected query result: %#v", series)
	}
}

func TestLokiClientParsesStructuredStreams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"status":"success","data":{"resultType":"streams","result":[{"stream":{"service_name":"luna-agent","trace_id":"trace-1"},"values":[["1720000000000000000","failed"]]}]}}`)
	}))
	defer server.Close()
	client, err := New(SourceLoki, Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	logs, err := client.QueryLogs(context.Background(), `{service_name="luna-agent"}`, time.Now().Add(-time.Hour), time.Now(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Labels["trace_id"] != "trace-1" || logs[0].Line != "failed" {
		t.Fatalf("unexpected logs: %#v", logs)
	}
}

func TestClientRejectsCredentialedQueryURL(t *testing.T) {
	if _, err := New(SourceTempo, Config{BaseURL: "https://user:pass@tempo.example.com"}); err == nil {
		t.Fatal("credentialed observability URL was accepted")
	}
}
