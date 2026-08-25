package agentobservability

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestTraceTokenUsageUsesWeightedReportedModelUsage(t *testing.T) {
	usage := aggregateTraceTokenUsage([]TraceSpan{
		{Attributes: map[string]string{
			"gen_ai.operation.name": "chat", "gen_ai.usage.input_tokens": "100", "gen_ai.usage.output_tokens": "10",
			"gen_ai.usage.cache_read.input_tokens": "20", "gen_ai.usage.cache_write.input_tokens": "5", "gen_ai.usage.reasoning.output_tokens": "2",
		}},
		{Attributes: map[string]string{
			"gen_ai.operation.name": "generate_content", "gen_ai.usage.input_tokens": "300", "gen_ai.usage.output_tokens": "20",
			"gen_ai.usage.cache_read.input_tokens": "30", "gen_ai.usage.cache_write.input_tokens": "10", "gen_ai.usage.reasoning.output_tokens": "3",
		}},
		{Attributes: map[string]string{
			"gen_ai.operation.name": "execute_tool", "gen_ai.usage.input_tokens": "999", "gen_ai.usage.output_tokens": "999",
			"gen_ai.usage.cache_read.input_tokens": "999",
		}},
		{Attributes: map[string]string{
			"gen_ai.operation.name": "chat", "luna.gen_ai.usage.status": "unavailable",
			"luna.gen_ai.usage.unavailable_reason": "missing_usage",
		}},
	})
	if usage == nil {
		t.Fatal("reported trace usage is nil")
	}
	if usage.InputTokens != 400 || usage.OutputTokens != 30 {
		t.Fatalf("trace totals = %#v", usage)
	}
	assertInt64Pointer(t, "trace cache read", usage.CacheReadInputTokens, 50)
	assertInt64Pointer(t, "trace cache write", usage.CacheWriteInputTokens, 15)
	assertInt64Pointer(t, "trace reasoning", usage.ReasoningOutputTokens, 5)
	if usage.CacheHitRate == nil || *usage.CacheHitRate != 12.5 {
		t.Fatalf("weighted trace cache hit rate = %#v, want 12.5", usage.CacheHitRate)
	}
}

func TestTempoTraceDetailBuildsTypedUsageFromNormalizedAttributes(t *testing.T) {
	const payload = `{"batches":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"luna-agent"}}]},"scopeSpans":[{"spans":[{"spanId":"model","name":"chat gpt-5","kind":"SPAN_KIND_CLIENT","startTimeUnixNano":"1000000000","endTimeUnixNano":"2000000000","attributes":[{"key":"gen_ai.operation.name","value":{"stringValue":"chat"}},{"key":"gen_ai.usage.input_tokens","value":{"intValue":"80"}},{"key":"gen_ai.usage.output_tokens","value":{"intValue":"20"}},{"key":"gen_ai.usage.cache_read.input_tokens","value":{"intValue":"20"}}],"status":{"code":"STATUS_CODE_OK"}}]}]}]}`
	var response tempoTraceResponse
	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		t.Fatal(err)
	}
	detail := tempoTraceDetail("0123456789abcdef0123456789abcdef", response)
	if detail.Usage == nil || detail.Usage.InputTokens != 80 || detail.Usage.OutputTokens != 20 {
		t.Fatalf("typed trace usage = %#v", detail.Usage)
	}
	assertInt64Pointer(t, "normalized trace cache read", detail.Usage.CacheReadInputTokens, 20)
	if detail.Usage.CacheHitRate == nil || *detail.Usage.CacheHitRate != 25 {
		t.Fatalf("normalized trace cache hit rate = %#v, want 25", detail.Usage.CacheHitRate)
	}
}

func TestTraceTokenUsagePreservesNullAndExplicitZero(t *testing.T) {
	zero := aggregateTraceTokenUsage([]TraceSpan{{Attributes: map[string]string{
		"gen_ai.operation.name": "text_completion", "gen_ai.usage.input_tokens": "100", "gen_ai.usage.output_tokens": "0",
		"gen_ai.usage.cache_read.input_tokens": "0",
	}}})
	if zero == nil || zero.CacheReadInputTokens == nil || *zero.CacheReadInputTokens != 0 || zero.CacheHitRate == nil || *zero.CacheHitRate != 0 {
		t.Fatalf("explicit zero trace cache usage was lost: %#v", zero)
	}
	if zero.CacheWriteInputTokens != nil || zero.ReasoningOutputTokens != nil {
		t.Fatalf("omitted trace breakdowns must remain null: %#v", zero)
	}

	missing := aggregateTraceTokenUsage([]TraceSpan{{Attributes: map[string]string{
		"gen_ai.operation.name": "chat", "gen_ai.usage.input_tokens": "100", "gen_ai.usage.output_tokens": "10",
	}}})
	if missing == nil || missing.CacheReadInputTokens != nil || missing.CacheHitRate != nil {
		t.Fatalf("missing cache-read usage must produce a null rate: %#v", missing)
	}

	zeroInput := aggregateTraceTokenUsage([]TraceSpan{{Attributes: map[string]string{
		"gen_ai.operation.name": "chat", "gen_ai.usage.input_tokens": "0", "gen_ai.usage.output_tokens": "0",
		"gen_ai.usage.cache_read.input_tokens": "0",
	}}})
	if zeroInput == nil || zeroInput.CacheReadInputTokens == nil || zeroInput.CacheHitRate != nil {
		t.Fatalf("zero input must keep explicit cache usage but leave the rate undefined: %#v", zeroInput)
	}
}

func TestTraceTokenUsageRejectsAmbiguousOrMalformedTotals(t *testing.T) {
	maxInt64 := strconv.FormatInt(math.MaxInt64, 10)
	tests := map[string][]TraceSpan{
		"no model usage":   {{Attributes: map[string]string{"gen_ai.operation.name": "execute_tool"}}},
		"unavailable only": {{Attributes: map[string]string{"gen_ai.operation.name": "chat", "luna.gen_ai.usage.status": "unavailable"}}},
		"missing output":   {{Attributes: map[string]string{"gen_ai.operation.name": "chat", "gen_ai.usage.input_tokens": "1"}}},
		"negative input":   {{Attributes: map[string]string{"gen_ai.operation.name": "chat", "gen_ai.usage.input_tokens": "-1", "gen_ai.usage.output_tokens": "1"}}},
		"unknown status":   {{Attributes: map[string]string{"gen_ai.operation.name": "chat", "luna.gen_ai.usage.status": "partial", "gen_ai.usage.input_tokens": "1", "gen_ai.usage.output_tokens": "1"}}},
		"overflow": {
			{Attributes: map[string]string{"gen_ai.operation.name": "chat", "gen_ai.usage.input_tokens": maxInt64, "gen_ai.usage.output_tokens": "0"}},
			{Attributes: map[string]string{"gen_ai.operation.name": "chat", "gen_ai.usage.input_tokens": "1", "gen_ai.usage.output_tokens": "0"}},
		},
	}
	for name, spans := range tests {
		t.Run(name, func(t *testing.T) {
			if usage := aggregateTraceTokenUsage(spans); usage != nil {
				t.Fatalf("invalid trace usage = %#v, want nil", usage)
			}
		})
	}
}

func TestTraceTokenUsageInvalidBreakdownDoesNotBecomeZero(t *testing.T) {
	usage := aggregateTraceTokenUsage([]TraceSpan{{Attributes: map[string]string{
		"gen_ai.operation.name": "chat", "gen_ai.usage.input_tokens": "10", "gen_ai.usage.output_tokens": "2",
		"gen_ai.usage.cache_read.input_tokens": "11", "gen_ai.usage.cache_write.input_tokens": "bad",
		"gen_ai.usage.reasoning.output_tokens": "3",
	}}})
	if usage == nil || usage.InputTokens != 10 || usage.OutputTokens != 2 {
		t.Fatalf("valid trace totals were lost: %#v", usage)
	}
	if usage.CacheReadInputTokens != nil || usage.CacheWriteInputTokens != nil || usage.ReasoningOutputTokens != nil || usage.CacheHitRate != nil {
		t.Fatalf("invalid trace breakdowns must remain null: %#v", usage)
	}
}

func TestTraceDetailUsageIsStableNullableJSON(t *testing.T) {
	payload, err := json.Marshal(TraceDetail{})
	if err != nil {
		t.Fatal(err)
	}
	var detail map[string]any
	if err := json.Unmarshal(payload, &detail); err != nil {
		t.Fatal(err)
	}
	if value, exists := detail["usage"]; !exists || value != nil {
		t.Fatalf("trace detail usage must be emitted as null: %s", payload)
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

func TestTraceAttributeAllowlistUsesCurrentUsageContract(t *testing.T) {
	for _, key := range []string{
		"gen_ai.usage.cache_write.input_tokens",
		"luna.gen_ai.usage.status",
		"luna.gen_ai.usage.unavailable_reason",
	} {
		if _, ok := traceAttributeAllowlist[key]; !ok {
			t.Fatalf("current usage attribute %q is not retained", key)
		}
	}
	for _, key := range []string{
		"gen_ai.usage.cache_creation.input_tokens",
		"luna.gen_ai.usage.reported",
	} {
		if _, ok := traceAttributeAllowlist[key]; ok {
			t.Fatalf("pre-release legacy usage attribute %q is still retained", key)
		}
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
