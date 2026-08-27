package agentobservability

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const maxResponseBytes = 16 << 20

const AgentTraceQuery = `{ resource.service.name = "luna-agent" }`

type Source string

const (
	SourcePrometheus Source = "prometheus"
	SourceLoki       Source = "loki"
	SourceTempo      Source = "tempo"
)

type Config struct {
	BaseURL  string
	Token    string
	TenantID string
}

type Client struct {
	baseURL *url.URL
	token   string
	tenant  string
	http    *http.Client
	source  Source
}

type TestResult struct {
	Source        Source `json:"source"`
	Reachable     bool   `json:"reachable"`
	DataAvailable bool   `json:"dataAvailable"`
	LatencyMS     int64  `json:"latencyMs"`
	Code          string `json:"code"`
}

type Point struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

type Series struct {
	Labels map[string]string `json:"labels"`
	Points []Point           `json:"points"`
}

type LogEntry struct {
	Timestamp string            `json:"timestamp"`
	Line      string            `json:"line"`
	Labels    map[string]string `json:"labels"`
}

type TraceSummary struct {
	TraceID         string `json:"traceId"`
	RootServiceName string `json:"rootServiceName"`
	RootTraceName   string `json:"rootTraceName"`
	StartTime       string `json:"startTimeUnixNano"`
	DurationMS      int64  `json:"durationMs"`
}

type TraceDetail struct {
	TraceID    string        `json:"traceId"`
	DurationMS float64       `json:"durationMs"`
	SpanCount  int           `json:"spanCount"`
	ErrorCount int           `json:"errorCount"`
	Usage      *TokenUsage   `json:"usage"`
	Spans      []TraceSpan   `json:"spans"`
	Context    *TraceContext `json:"context,omitempty"`
}

type TraceSpan struct {
	SpanID         string            `json:"spanId"`
	ParentSpanID   string            `json:"parentSpanId"`
	Name           string            `json:"name"`
	ServiceName    string            `json:"serviceName"`
	Kind           string            `json:"kind"`
	Status         string            `json:"status"`
	StartTimeNanos string            `json:"startTimeUnixNano"`
	StartOffsetMS  float64           `json:"startOffsetMs"`
	DurationMS     float64           `json:"durationMs"`
	Attributes     map[string]string `json:"attributes"`
	Events         []TraceSpanEvent  `json:"events"`
	Raw            json.RawMessage   `json:"raw"`
}

type TraceSpanEvent struct {
	Name          string            `json:"name"`
	TimeUnixNanos string            `json:"timeUnixNano"`
	Attributes    map[string]string `json:"attributes"`
}

func New(source Source, config Config) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(config.BaseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("invalid %s query URL", source)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	httpClient := telemetry.InstrumentHTTPClient(&http.Client{
		Timeout:   12 * time.Second,
		Transport: transport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 3 || !sameOrigin(parsed, request.URL) {
				return http.ErrUseLastResponse
			}
			return nil
		},
	})
	return &Client{baseURL: parsed, token: strings.TrimSpace(config.Token), tenant: strings.TrimSpace(config.TenantID), http: httpClient, source: source}, nil
}

func (c *Client) Test(ctx context.Context) (result TestResult, err error) {
	startedAt := time.Now()
	result.Source = c.source
	defer func() { result.LatencyMS = time.Since(startedAt).Milliseconds() }()
	switch c.source {
	case SourcePrometheus:
		series, queryErr := c.Query(ctx, "sum(luna_devops_agent_runs_total) or vector(0)", time.Time{})
		err = queryErr
		result.DataAvailable = len(series) > 0 && len(series[0].Points) > 0 && series[0].Points[0].Value > 0
	case SourceLoki:
		logs, queryErr := c.QueryLogs(ctx, `{service_name="luna-agent"}`, time.Now().Add(-time.Hour), time.Now(), 1)
		err = queryErr
		result.DataAvailable = len(logs) > 0
	case SourceTempo:
		traces, queryErr := c.SearchTraces(ctx, AgentTraceQuery, time.Now().Add(-time.Hour), time.Now(), 1)
		err = queryErr
		result.DataAvailable = len(traces) > 0
	default:
		err = fmt.Errorf("unsupported observability source")
	}
	if err != nil {
		result.Code = "ai.observability.connection_failed"
		return result, err
	}
	result.Reachable = true
	if result.DataAvailable {
		result.Code = "ai.observability.data_available"
	} else {
		result.Code = "ai.observability.no_recent_data"
	}
	return result, nil
}

func (c *Client) Query(ctx context.Context, query string, at time.Time) ([]Series, error) {
	params := url.Values{"query": []string{query}}
	if !at.IsZero() {
		params.Set("time", strconv.FormatInt(at.Unix(), 10))
	}
	var response prometheusResponse
	if err := c.getJSON(ctx, "prometheus.query", "/api/v1/query", params, &response); err != nil {
		return nil, err
	}
	if response.Status != "success" {
		return nil, fmt.Errorf("Prometheus query failed")
	}
	return prometheusSeries(response.Data.Result), nil
}

func (c *Client) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]Series, error) {
	params := url.Values{
		"query": []string{query},
		"start": []string{strconv.FormatInt(start.Unix(), 10)},
		"end":   []string{strconv.FormatInt(end.Unix(), 10)},
		"step":  []string{strconv.FormatInt(maxInt64(1, int64(step.Seconds())), 10)},
	}
	var response prometheusResponse
	if err := c.getJSON(ctx, "prometheus.query_range", "/api/v1/query_range", params, &response); err != nil {
		return nil, err
	}
	if response.Status != "success" {
		return nil, fmt.Errorf("Prometheus range query failed")
	}
	return prometheusSeries(response.Data.Result), nil
}

func (c *Client) QueryLogs(ctx context.Context, query string, start, end time.Time, limit int) ([]LogEntry, error) {
	params := url.Values{
		"query":     []string{query},
		"start":     []string{strconv.FormatInt(start.UnixNano(), 10)},
		"end":       []string{strconv.FormatInt(end.UnixNano(), 10)},
		"limit":     []string{strconv.Itoa(limit)},
		"direction": []string{"backward"},
	}
	var response lokiResponse
	if err := c.getJSON(ctx, "loki.query_range", "/loki/api/v1/query_range", params, &response); err != nil {
		return nil, err
	}
	if response.Status != "success" {
		return nil, fmt.Errorf("Loki query failed")
	}
	entries := make([]LogEntry, 0)
	for _, stream := range response.Data.Result {
		for _, value := range stream.Values {
			if len(value) == 2 {
				entries = append(entries, LogEntry{Timestamp: value[0], Line: truncateRunes(value[1], 4096), Labels: stream.Stream})
			}
		}
	}
	return entries, nil
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func (c *Client) SearchTraces(ctx context.Context, query string, start, end time.Time, limit int) ([]TraceSummary, error) {
	params := url.Values{
		"q":     []string{query},
		"start": []string{strconv.FormatInt(start.Unix(), 10)},
		"end":   []string{strconv.FormatInt(end.Unix(), 10)},
		"limit": []string{strconv.Itoa(limit)},
	}
	var response tempoSearchResponse
	if err := c.getJSON(ctx, "tempo.search", "/api/search", params, &response); err != nil {
		return nil, err
	}
	return response.Traces, nil
}

func (c *Client) GetTrace(ctx context.Context, traceID string) (TraceDetail, error) {
	traceID = strings.TrimSpace(traceID)
	if len(traceID) != 32 {
		return TraceDetail{}, fmt.Errorf("invalid trace ID")
	}
	if _, err := strconv.ParseUint(traceID[:16], 16, 64); err != nil {
		return TraceDetail{}, fmt.Errorf("invalid trace ID")
	}
	if _, err := strconv.ParseUint(traceID[16:], 16, 64); err != nil {
		return TraceDetail{}, fmt.Errorf("invalid trace ID")
	}
	var response tempoTraceResponse
	if err := c.getJSON(ctx, "tempo.trace.get", "/api/v2/traces/"+traceID, nil, &response); err != nil {
		return TraceDetail{}, err
	}
	detail := tempoTraceDetail(traceID, response)
	if detail.SpanCount == 0 {
		return TraceDetail{}, fmt.Errorf("Tempo trace response contains no spans")
	}
	return detail, nil
}

func (c *Client) getJSON(ctx context.Context, operation, path string, params url.Values, target any) (err error) {
	ctx, end := telemetry.StartOperationWithKind(ctx, "agent_observability", operation, trace.SpanKindClient,
		attribute.String("observability.source", string(c.source)))
	defer func() { end(err) }()
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	endpoint.RawQuery = params.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.tenant != "" {
		request.Header.Set("X-Scope-OrgID", c.tenant)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("%s returned status %d", c.source, response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s response: %w", c.source, err)
	}
	return nil
}

type prometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Metric map[string]string   `json:"metric"`
			Value  []json.RawMessage   `json:"value"`
			Values [][]json.RawMessage `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

type lokiResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

type tempoSearchResponse struct {
	Traces []TraceSummary `json:"traces"`
}

type tempoTraceResponse struct {
	// Tempo v2 wraps the OTLP payload in trace.resourceSpans. Batches keeps
	// compatibility with the legacy JSON representation returned by v1 proxies.
	Trace struct {
		ResourceSpans []tempoResourceSpans `json:"resourceSpans"`
	} `json:"trace"`
	Batches []tempoResourceSpans `json:"batches"`
}

type tempoResourceSpans struct {
	Resource struct {
		Attributes []tempoAttribute `json:"attributes"`
	} `json:"resource"`
	ScopeSpans []struct {
		Spans []tempoSpan `json:"spans"`
	} `json:"scopeSpans"`
	InstrumentationLibrarySpans []struct {
		Spans []tempoSpan `json:"spans"`
	} `json:"instrumentationLibrarySpans"`
}

type tempoSpan struct {
	SpanID            string           `json:"spanId"`
	ParentSpanID      string           `json:"parentSpanId"`
	Name              string           `json:"name"`
	Kind              string           `json:"kind"`
	StartTimeUnixNano string           `json:"startTimeUnixNano"`
	EndTimeUnixNano   string           `json:"endTimeUnixNano"`
	Attributes        []tempoAttribute `json:"attributes"`
	Events            []tempoSpanEvent `json:"events"`
	Status            struct {
		Code string `json:"code"`
	} `json:"status"`
	Raw json.RawMessage `json:"-"`
}

type tempoSpanEvent struct {
	Name         string           `json:"name"`
	TimeUnixNano string           `json:"timeUnixNano"`
	Attributes   []tempoAttribute `json:"attributes"`
}

type tempoAttribute struct {
	Key   string        `json:"key"`
	Value tempoAnyValue `json:"value"`
}

type tempoAnyValue struct {
	StringValue *string          `json:"stringValue"`
	IntValue    *string          `json:"intValue"`
	DoubleValue *float64         `json:"doubleValue"`
	BoolValue   *bool            `json:"boolValue"`
	ArrayValue  *tempoArrayValue `json:"arrayValue"`
	KVListValue *tempoKVList     `json:"kvlistValue"`
}

type tempoArrayValue struct {
	Values []tempoAnyValue `json:"values"`
}

type tempoKVList struct {
	Values []tempoAttribute `json:"values"`
}

func (span *tempoSpan) UnmarshalJSON(data []byte) error {
	type rawTempoSpan tempoSpan
	var decoded rawTempoSpan
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*span = tempoSpan(decoded)
	span.Raw = append(json.RawMessage(nil), data...)
	return nil
}

var traceAttributeAllowlist = map[string]struct{}{
	"gen_ai.operation.name": {}, "gen_ai.provider.name": {}, "gen_ai.agent.name": {},
	"gen_ai.agent.description": {}, "gen_ai.agent.version": {}, "gen_ai.conversation.id": {},
	"gen_ai.conversation.compacted": {}, "gen_ai.output.type": {},
	"gen_ai.request.model": {}, "gen_ai.request.max_tokens": {}, "gen_ai.request.stream": {},
	"gen_ai.request.reasoning.level": {},
	"gen_ai.response.id":             {}, "gen_ai.response.model": {}, "gen_ai.response.finish_reasons": {},
	"gen_ai.response.time_to_first_chunk": {},
	"gen_ai.usage.input_tokens":           {}, "gen_ai.usage.output_tokens": {},
	"gen_ai.usage.cache_read.input_tokens": {}, "gen_ai.usage.cache_write.input_tokens": {},
	"gen_ai.usage.reasoning.output_tokens": {},
	"gen_ai.system_instructions":           {}, "gen_ai.input.messages": {}, "gen_ai.output.messages": {}, "gen_ai.tool.definitions": {},
	"gen_ai.tool.name": {}, "gen_ai.tool.call.id": {}, "gen_ai.tool.description": {}, "gen_ai.tool.type": {},
	"gen_ai.tool.call.arguments": {}, "gen_ai.tool.call.result": {},
	"openai.response.service_tier": {}, "openai.response.system_fingerprint": {},
	"server.address": {}, "server.port": {}, "luna.turn.id": {}, "luna.run.id": {},
	"luna.tool_call.id": {}, "luna.tool_call.count": {}, "luna.ai.content.truncated": {},
	"luna.gen_ai.request.purpose": {}, "luna.gen_ai.usage.status": {}, "luna.gen_ai.usage.unavailable_reason": {},
	"luna.gen_ai.response.error_body": {}, "luna.operation.name": {},
	"http.request.method": {}, "http.response.status_code": {},
	"db.system.name": {}, "error.type": {}, "luna.run.outcome": {},
}

func tempoTraceDetail(traceID string, response tempoTraceResponse) TraceDetail {
	detail := TraceDetail{TraceID: traceID, Spans: []TraceSpan{}}
	var earliest, latest int64
	batches := response.Trace.ResourceSpans
	if len(batches) == 0 {
		batches = response.Batches
	}
	for _, batch := range batches {
		resourceAttributes := tempoAttributes(batch.Resource.Attributes, nil)
		serviceName := resourceAttributes["service.name"]
		groups := make([][]tempoSpan, 0, len(batch.ScopeSpans)+len(batch.InstrumentationLibrarySpans))
		for _, group := range batch.ScopeSpans {
			groups = append(groups, group.Spans)
		}
		for _, group := range batch.InstrumentationLibrarySpans {
			groups = append(groups, group.Spans)
		}
		for _, spans := range groups {
			for _, span := range spans {
				start, _ := strconv.ParseInt(span.StartTimeUnixNano, 10, 64)
				end, _ := strconv.ParseInt(span.EndTimeUnixNano, 10, 64)
				if earliest == 0 || start < earliest {
					earliest = start
				}
				if end > latest {
					latest = end
				}
				status := strings.TrimPrefix(strings.ToLower(span.Status.Code), "status_code_")
				if status == "" {
					status = "unset"
				}
				if status == "error" {
					detail.ErrorCount++
				}
				events := make([]TraceSpanEvent, 0, len(span.Events))
				for _, event := range span.Events {
					events = append(events, TraceSpanEvent{
						Name: event.Name, TimeUnixNanos: event.TimeUnixNano,
						Attributes: tempoAttributes(event.Attributes, nil),
					})
				}
				raw := span.Raw
				if len(raw) == 0 {
					raw = json.RawMessage(`{}`)
				}
				detail.Spans = append(detail.Spans, TraceSpan{
					SpanID: span.SpanID, ParentSpanID: span.ParentSpanID, Name: tempoSpanName(span.Name),
					ServiceName: serviceName, Kind: strings.TrimPrefix(strings.ToLower(span.Kind), "span_kind_"),
					Status: status, StartTimeNanos: span.StartTimeUnixNano, DurationMS: float64(end-start) / 1e6,
					Attributes: tempoAttributes(span.Attributes, traceAttributeAllowlist),
					Events:     events, Raw: raw,
				})
			}
		}
	}
	detail.SpanCount = len(detail.Spans)
	detail.DurationMS = float64(latest-earliest) / 1e6
	for index := range detail.Spans {
		start, _ := strconv.ParseInt(detail.Spans[index].StartTimeNanos, 10, 64)
		detail.Spans[index].StartOffsetMS = float64(start-earliest) / 1e6
	}
	detail.Usage = aggregateTraceTokenUsage(detail.Spans)
	return detail
}

func tempoSpanName(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "handler - ") {
		return "fastify.handler"
	}
	return truncateRunes(name, 160)
}

func tempoAttributes(attributes []tempoAttribute, allowlist map[string]struct{}) map[string]string {
	result := map[string]string{}
	for _, attribute := range attributes {
		if allowlist != nil {
			if _, ok := allowlist[attribute.Key]; !ok {
				continue
			}
		}
		if value, ok := tempoAttributeString(attribute.Value); ok {
			result[attribute.Key] = value
		}
	}
	return result
}

func tempoAttributeString(value tempoAnyValue) (string, bool) {
	switch {
	case value.StringValue != nil:
		return *value.StringValue, true
	case value.IntValue != nil:
		return *value.IntValue, true
	case value.DoubleValue != nil:
		return strconv.FormatFloat(*value.DoubleValue, 'f', -1, 64), true
	case value.BoolValue != nil:
		return strconv.FormatBool(*value.BoolValue), true
	case value.ArrayValue != nil, value.KVListValue != nil:
		normalized, ok := tempoAttributeJSONValue(value)
		if !ok {
			return "", false
		}
		encoded, err := json.Marshal(normalized)
		return string(encoded), err == nil
	default:
		return "", false
	}
}

func tempoAttributeJSONValue(value tempoAnyValue) (any, bool) {
	switch {
	case value.StringValue != nil:
		return *value.StringValue, true
	case value.IntValue != nil:
		parsed, err := strconv.ParseInt(*value.IntValue, 10, 64)
		if err != nil {
			return *value.IntValue, true
		}
		return parsed, true
	case value.DoubleValue != nil:
		return *value.DoubleValue, true
	case value.BoolValue != nil:
		return *value.BoolValue, true
	case value.ArrayValue != nil:
		items := make([]any, 0, len(value.ArrayValue.Values))
		for _, item := range value.ArrayValue.Values {
			normalized, ok := tempoAttributeJSONValue(item)
			if ok {
				items = append(items, normalized)
			}
		}
		return items, true
	case value.KVListValue != nil:
		items := make(map[string]any, len(value.KVListValue.Values))
		for _, item := range value.KVListValue.Values {
			normalized, ok := tempoAttributeJSONValue(item.Value)
			if ok {
				items[item.Key] = normalized
			}
		}
		return items, true
	default:
		return nil, false
	}
}

func prometheusSeries(items []struct {
	Metric map[string]string   `json:"metric"`
	Value  []json.RawMessage   `json:"value"`
	Values [][]json.RawMessage `json:"values"`
}) []Series {
	result := make([]Series, 0, len(items))
	for _, item := range items {
		series := Series{Labels: item.Metric, Points: make([]Point, 0, len(item.Values)+1)}
		if point, ok := prometheusPoint(item.Value); ok {
			series.Points = append(series.Points, point)
		}
		for _, raw := range item.Values {
			if point, ok := prometheusPoint(raw); ok {
				series.Points = append(series.Points, point)
			}
		}
		result = append(result, series)
	}
	return result
}

func prometheusPoint(raw []json.RawMessage) (Point, bool) {
	if len(raw) != 2 {
		return Point{}, false
	}
	var timestamp float64
	var valueText string
	if json.Unmarshal(raw[0], &timestamp) != nil || json.Unmarshal(raw[1], &valueText) != nil {
		return Point{}, false
	}
	value, err := strconv.ParseFloat(valueText, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return Point{}, false
	}
	return Point{Timestamp: int64(timestamp), Value: value}, true
}

func sameOrigin(left, right *url.URL) bool {
	return left.Scheme == right.Scheme && strings.EqualFold(left.Host, right.Host)
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
