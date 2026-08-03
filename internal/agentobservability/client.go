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

const maxResponseBytes = 4 << 20

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
