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

func TestTempoTraceDetailNormalizesAgentSpans(t *testing.T) {
	const payload = `{"batches":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"luna-agent"}}]},"scopeSpans":[{"spans":[{"spanId":"root","name":"invoke_agent Luna Agent","kind":"SPAN_KIND_INTERNAL","startTimeUnixNano":"1000000000","endTimeUnixNano":"2000000000","attributes":[{"key":"gen_ai.operation.name","value":{"stringValue":"invoke_agent"}},{"key":"gen_ai.agent.name","value":{"stringValue":"Luna Agent"}}],"status":{"code":"STATUS_CODE_OK"}},{"spanId":"model","parentSpanId":"root","name":"chat gpt-5","kind":"SPAN_KIND_CLIENT","startTimeUnixNano":"1100000000","endTimeUnixNano":"1900000000","attributes":[{"key":"gen_ai.operation.name","value":{"stringValue":"chat"}},{"key":"gen_ai.usage.input_tokens","value":{"intValue":"42"}},{"key":"gen_ai.tool.call.id","value":{"stringValue":"tool-call-1"}},{"key":"gen_ai.response.finish_reasons","value":{"arrayValue":{"values":[{"stringValue":"tool_call"}]} }},{"key":"gen_ai.input.messages","value":{"stringValue":"[{\"role\":\"user\",\"parts\":[{\"type\":\"text\",\"content\":\"Deploy it\"}]}]"}},{"key":"gen_ai.output.messages","value":{"stringValue":"[{\"role\":\"assistant\",\"parts\":[{\"type\":\"text\",\"content\":\"Done\"}],\"finish_reason\":\"stop\"}]"}}],"status":{"code":"STATUS_CODE_ERROR"}}]}]}]}`
	var response tempoTraceResponse
	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	detail := tempoTraceDetail("0123456789abcdef0123456789abcdef", response)
	if detail.SpanCount != 2 || detail.ErrorCount != 1 || detail.DurationMS != 1000 {
		t.Fatalf("unexpected trace summary: %+v", detail)
	}
	if detail.Spans[1].StartOffsetMS != 100 || detail.Spans[1].Attributes["gen_ai.usage.input_tokens"] != "42" {
		t.Fatalf("model span was not normalized: %+v", detail.Spans[1])
	}
	if detail.Spans[1].Attributes["gen_ai.tool.call.id"] != "tool-call-1" {
		t.Fatalf("tool call correlation attribute was not retained: %+v", detail.Spans[1])
	}
	if detail.Spans[1].Attributes["gen_ai.input.messages"] == "" || detail.Spans[1].Attributes["gen_ai.output.messages"] == "" {
		t.Fatalf("schema-shaped model content was not retained: %+v", detail.Spans[1])
	}
	if detail.Spans[1].Attributes["gen_ai.response.finish_reasons"] != `["tool_call"]` {
		t.Fatalf("array-valued semantic attribute was not normalized: %+v", detail.Spans[1].Attributes)
	}
	var raw map[string]any
	if err := json.Unmarshal(detail.Spans[1].Raw, &raw); err != nil || raw["attributes"] == nil {
		t.Fatalf("raw Tempo span was not retained: raw=%s err=%v", detail.Spans[1].Raw, err)
	}
}

func TestTempoTraceDetailRetainsAvailableToolInventory(t *testing.T) {
	const payload = `{"batches":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"luna-agent"}}]},"scopeSpans":[{"spans":[{"spanId":"tools","name":"agent.tools.available","kind":"SPAN_KIND_INTERNAL","startTimeUnixNano":"1000000000","endTimeUnixNano":"1000001000","attributes":[{"key":"luna.agent.available_tool.count","value":{"intValue":"2"}},{"key":"luna.agent.available_tool.names","value":{"stringValue":"[\"createGatewayRoute\",\"listGatewayRoutes\"]"}}],"status":{"code":"STATUS_CODE_OK"}}]}]}]}`
	var response tempoTraceResponse
	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		t.Fatal(err)
	}
	detail := tempoTraceDetail("0123456789abcdef0123456789abcdef", response)
	if len(detail.Spans) != 1 {
		t.Fatalf("unexpected spans: %+v", detail.Spans)
	}
	if got := detail.Spans[0].Attributes["luna.agent.available_tool.names"]; got != `["createGatewayRoute","listGatewayRoutes"]` {
		t.Fatalf("available tool inventory was lost: %q", got)
	}
}

func TestTempoAttributesNormalizesStructuredAnyValues(t *testing.T) {
	finishReason := "stop"
	projectID := "proj-1"
	attributes := []tempoAttribute{
		{
			Key: "gen_ai.response.finish_reasons",
			Value: tempoAnyValue{ArrayValue: &tempoArrayValue{Values: []tempoAnyValue{
				{StringValue: &finishReason},
			}}},
		},
		{
			Key: "gen_ai.tool.call.arguments",
			Value: tempoAnyValue{KVListValue: &tempoKVList{Values: []tempoAttribute{
				{Key: "projectId", Value: tempoAnyValue{StringValue: &projectID}},
			}}},
		},
	}

	got := tempoAttributes(attributes, traceAttributeAllowlist)
	if got["gen_ai.response.finish_reasons"] != `["stop"]` {
		t.Fatalf("unexpected finish reasons: %q", got["gen_ai.response.finish_reasons"])
	}
	if got["gen_ai.tool.call.arguments"] != `{"projectId":"proj-1"}` {
		t.Fatalf("unexpected structured tool arguments: %q", got["gen_ai.tool.call.arguments"])
	}
}

func TestTempoTraceDetailParsesTempoV2ResourceSpans(t *testing.T) {
	const payload = `{"trace":{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"luna-agent"}}]},"scopeSpans":[{"spans":[{"spanId":"cm9vdA==","name":"handler - async secured => { /* generated handler source */ }","kind":"SPAN_KIND_INTERNAL","startTimeUnixNano":"1000000000","endTimeUnixNano":"2500000000","status":{"code":"STATUS_CODE_OK"}}]}]}]},"metrics":{"inspectedBytes":"1024"}}`
	var response tempoTraceResponse
	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		t.Fatalf("decode Tempo v2 fixture: %v", err)
	}
	detail := tempoTraceDetail("0123456789abcdef0123456789abcdef", response)
	if detail.SpanCount != 1 || detail.DurationMS != 1500 {
		t.Fatalf("unexpected Tempo v2 trace detail: %+v", detail)
	}
	if detail.Spans[0].ServiceName != "luna-agent" || detail.Spans[0].Name != "fastify.handler" {
		t.Fatalf("Tempo v2 span was not normalized: %+v", detail.Spans[0])
	}
}

func TestTempoClientRejectsTraceResponseWithoutSpans(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"trace":{"resourceSpans":[]}}`)
	}))
	defer server.Close()
	client, err := New(SourceTempo, Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetTrace(context.Background(), "0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("empty Tempo trace response was accepted")
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
