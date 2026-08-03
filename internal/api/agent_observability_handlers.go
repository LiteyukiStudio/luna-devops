package api

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/agentobservability"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

const (
	agentObservabilityEnabledKey = "ai.observability.enabled"
	observabilityLogLimit        = 80
	observabilityTraceLimit      = 50
)

var observabilitySourceKeys = map[agentobservability.Source]struct {
	URL      string
	Token    string
	TenantID string
}{
	agentobservability.SourcePrometheus: {URL: "ai.observability.prometheus_url", Token: "ai.observability.prometheus_token"},
	agentobservability.SourceLoki:       {URL: "ai.observability.loki_url", Token: "ai.observability.loki_token", TenantID: "ai.observability.loki_tenant_id"},
	agentobservability.SourceTempo:      {URL: "ai.observability.tempo_url", Token: "ai.observability.tempo_token", TenantID: "ai.observability.tempo_tenant_id"},
}

type observabilityTestInput struct {
	Source   agentobservability.Source `json:"source" binding:"required"`
	URL      string                    `json:"url" binding:"required"`
	Token    string                    `json:"token"`
	TenantID string                    `json:"tenantId"`
}

type agentObservabilityOverview struct {
	GeneratedAt     time.Time                              `json:"generatedAt"`
	Range           string                                 `json:"range"`
	Summary         map[string]float64                     `json:"summary"`
	Series          map[string][]agentobservability.Series `json:"series"`
	Tools           []agentobservability.Series            `json:"tools"`
	Logs            []agentobservability.LogEntry          `json:"logs"`
	Traces          []agentobservability.TraceSummary      `json:"traces"`
	SourceStatus    map[agentobservability.Source]string   `json:"sourceStatus"`
	ObservationCode string                                 `json:"observationCode"`
}

func (h *Handlers) TestAgentObservabilitySource(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	var input observabilityTestInput
	if !bindJSON(ctx, &input) {
		return
	}
	keys, valid := observabilitySourceKeys[input.Source]
	if !valid {
		writeErrorCode(ctx, http.StatusBadRequest, "ai.observability.source_invalid", "unsupported observability source")
		return
	}
	token := strings.TrimSpace(input.Token)
	if token == "" {
		token = h.resolveAppConfigSecret(ctx, keys.Token)
	}
	client, err := agentobservability.New(input.Source, agentobservability.Config{
		BaseURL: input.URL, Token: token, TenantID: input.TenantID,
	})
	if err != nil {
		ctx.JSON(http.StatusOK, agentobservability.TestResult{Source: input.Source, Code: "ai.observability.config_invalid"})
		return
	}
	result, testErr := client.Test(ctx.Request.Context())
	h.auditWithContext(user.ID, "ai.observability.test", string(input.Source), testErr == nil, "Agent observability source tested", ctx.Request.Context())
	ctx.Header("Cache-Control", "no-store")
	// A failed test is a diagnostic result, not a failed configuration mutation.
	ctx.JSON(http.StatusOK, result)
}

func (h *Handlers) GetAgentObservabilityOverview(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return
	}
	values := h.configs.get(knownConfigKeys())
	if !configBool(values[agentObservabilityEnabledKey]) {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.observability.disabled", "Agent observability is disabled")
		return
	}
	rangeText, duration := observabilityRange(ctx.Query("range"))
	end := time.Now()
	start := end.Add(-duration)
	step := observabilityStep(duration)

	clients := make(map[agentobservability.Source]*agentobservability.Client, len(observabilitySourceKeys))
	for source, keys := range observabilitySourceKeys {
		client, err := agentobservability.New(source, agentobservability.Config{
			BaseURL: values[keys.URL], Token: h.resolveAppConfigSecret(ctx, keys.Token), TenantID: values[keys.TenantID],
		})
		if err != nil {
			writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.observability.not_configured", "Agent observability source is not configured")
			return
		}
		clients[source] = client
	}

	result := agentObservabilityOverview{
		GeneratedAt: end, Range: rangeText, Summary: map[string]float64{},
		Series: map[string][]agentobservability.Series{}, SourceStatus: map[agentobservability.Source]string{},
		ObservationCode: "ai.observability.ready",
	}
	var mutex sync.Mutex
	var wait sync.WaitGroup
	querySeries := func(key, query string) {
		defer wait.Done()
		series, err := clients[agentobservability.SourcePrometheus].QueryRange(ctx.Request.Context(), query, start, end, step)
		mutex.Lock()
		defer mutex.Unlock()
		if err != nil {
			result.SourceStatus[agentobservability.SourcePrometheus] = "unavailable"
			result.ObservationCode = "ai.observability.partial"
			return
		}
		result.Series[key] = series
	}
	querySummary := func(key, query string) {
		defer wait.Done()
		series, err := clients[agentobservability.SourcePrometheus].Query(ctx.Request.Context(), query, end)
		mutex.Lock()
		defer mutex.Unlock()
		if err != nil {
			result.SourceStatus[agentobservability.SourcePrometheus] = "unavailable"
			result.ObservationCode = "ai.observability.partial"
			return
		}
		result.Summary[key] = firstSeriesValue(series)
	}

	rateWindow := "5m"
	seriesQueries := map[string]string{
		"runRate":         fmt.Sprintf(`sum(rate(luna_devops_agent_runs_total[%s]))`, rateWindow),
		"runSuccessRate":  fmt.Sprintf(`100 * sum(rate(luna_devops_agent_runs_total{outcome=~"completed|succeeded"}[%s])) / clamp_min(sum(rate(luna_devops_agent_runs_total[%s])), 0.001)`, rateWindow, rateWindow),
		"firstTokenP95":   fmt.Sprintf(`histogram_quantile(0.95, sum(rate(luna_devops_agent_model_first_token_duration_seconds_bucket[%s])) by (le))`, rateWindow),
		"modelLatencyP95": fmt.Sprintf(`histogram_quantile(0.95, sum(rate(luna_devops_agent_model_request_duration_seconds_bucket[%s])) by (le))`, rateWindow),
		"tokenRate":       fmt.Sprintf(`sum(rate(luna_devops_agent_model_tokens_total[%s])) by (direction)`, rateWindow),
		"toolFailureRate": fmt.Sprintf(`sum(rate(luna_devops_agent_tool_calls_total{outcome!="success"}[%s])) by (tool)`, rateWindow),
	}
	summaryQueries := map[string]string{
		"activeRuns":        `sum(luna_devops_agent_active_runs) or vector(0)`,
		"runSuccessRate":    fmt.Sprintf(`100 * sum(rate(luna_devops_agent_runs_total{outcome=~"completed|succeeded"}[%s])) / clamp_min(sum(rate(luna_devops_agent_runs_total[%s])), 0.001)`, rateWindow, rateWindow),
		"modelErrorRate":    fmt.Sprintf(`(100 * sum(rate(luna_devops_agent_model_requests_total{outcome!="success"}[%s])) / clamp_min(sum(rate(luna_devops_agent_model_requests_total[%s])), 0.001)) or vector(0)`, rateWindow, rateWindow),
		"firstTokenP95":     fmt.Sprintf(`histogram_quantile(0.95, sum(rate(luna_devops_agent_model_first_token_duration_seconds_bucket[%s])) by (le))`, rateWindow),
		"outputTokenRate":   fmt.Sprintf(`sum(rate(luna_devops_agent_model_tokens_total{direction="output"}[%s])) or vector(0)`, rateWindow),
		"externalErrorRate": fmt.Sprintf(`sum(rate(luna_devops_agent_external_requests_total{outcome!="success"}[%s])) or vector(0)`, rateWindow),
	}
	for key, query := range seriesQueries {
		wait.Add(1)
		go querySeries(key, query)
	}
	for key, query := range summaryQueries {
		wait.Add(1)
		go querySummary(key, query)
	}
	wait.Add(2)
	go func() {
		defer wait.Done()
		logs, err := clients[agentobservability.SourceLoki].QueryLogs(ctx.Request.Context(), `{service_name="luna-agent"} | event_name=~`+"`"+`agent\.(run|model|tool)\.failed|gen_ai\.content\.error`+"`", start, end, observabilityLogLimit)
		mutex.Lock()
		defer mutex.Unlock()
		if err != nil {
			result.SourceStatus[agentobservability.SourceLoki] = "unavailable"
			result.ObservationCode = "ai.observability.partial"
			return
		}
		result.Logs = logs
		result.SourceStatus[agentobservability.SourceLoki] = "ready"
	}()
	go func() {
		defer wait.Done()
		traces, err := clients[agentobservability.SourceTempo].SearchTraces(ctx.Request.Context(), agentobservability.AgentTraceQuery, start, end, observabilityTraceLimit)
		mutex.Lock()
		defer mutex.Unlock()
		if err != nil {
			result.SourceStatus[agentobservability.SourceTempo] = "unavailable"
			result.ObservationCode = "ai.observability.partial"
			return
		}
		result.Traces = traces
		result.SourceStatus[agentobservability.SourceTempo] = "ready"
	}()
	wait.Wait()
	if _, exists := result.SourceStatus[agentobservability.SourcePrometheus]; !exists {
		result.SourceStatus[agentobservability.SourcePrometheus] = "ready"
	}
	result.Tools = result.Series["toolFailureRate"]
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, result)
}

func (h *Handlers) resolveAppConfigSecret(ctx *gin.Context, key string) string {
	if key == "" {
		return ""
	}
	var row model.AppConfig
	if err := h.dbFor(ctx).First(&row, "key = ?", key).Error; err != nil {
		return ""
	}
	return h.secrets.ResolveContext(ctx.Request.Context(), row.Value)
}

func observabilityRange(value string) (string, time.Duration) {
	switch value {
	case "6h":
		return "6h", 6 * time.Hour
	case "24h":
		return "24h", 24 * time.Hour
	default:
		return "1h", time.Hour
	}
}

func observabilityStep(duration time.Duration) time.Duration {
	if duration >= 24*time.Hour {
		return 5 * time.Minute
	}
	if duration >= 6*time.Hour {
		return time.Minute
	}
	return 15 * time.Second
}

func firstSeriesValue(series []agentobservability.Series) float64 {
	if len(series) == 0 || len(series[0].Points) == 0 {
		return 0
	}
	return series[0].Points[len(series[0].Points)-1].Value
}
