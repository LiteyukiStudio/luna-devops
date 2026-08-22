package gatewayprobe

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
)

type Collector struct {
	config     Config
	discoverer RouteDiscoverer
	reporter   Reporter
	client     *http.Client
	logger     *slog.Logger

	mu          sync.RWMutex
	routes      []RouteRef
	routeLoaded time.Time
	states      map[string]routeState
	lastError   string
	lastScrape  time.Time
	lastReport  time.Time
}

func NewCollector(config Config, discoverer RouteDiscoverer, reporter Reporter, logger *slog.Logger) *Collector {
	if logger == nil {
		logger = slog.Default()
	}
	return &Collector{
		config:     config,
		discoverer: discoverer,
		reporter:   reporter,
		client:     telemetry.InstrumentHTTPClient(&http.Client{Timeout: config.HTTPTimeout}),
		logger:     logger,
		states:     map[string]routeState{},
	}
}

func (c *Collector) Run(ctx context.Context) error {
	if err := c.refreshRoutes(ctx); err != nil {
		c.setError(err)
		c.logWarning(ctx, "Initial route refresh failed", "gateway_probe.routes.refresh_failed", "gateway_probe.routes.refresh", err)
	}
	if err := c.scrapeAndReport(ctx); err != nil {
		c.setError(err)
		c.logWarning(ctx, "Initial gateway traffic scrape failed", "gateway_probe.scrape.failed", "gateway_probe.scrape", err)
	}
	ticker := time.NewTicker(c.config.ScrapeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := c.scrapeAndReport(ctx); err != nil {
				c.setError(err)
				c.logWarning(ctx, "Gateway traffic scrape failed", "gateway_probe.scrape.failed", "gateway_probe.scrape", err)
			}
		}
	}
}

func (c *Collector) logWarning(ctx context.Context, message, eventName, operation string, err error) {
	attrs := []slog.Attr{
		slog.String("event.name", eventName),
		slog.String("operation", operation),
		slog.String("outcome", telemetry.ErrorOutcome(err)),
	}
	attrs = append(attrs, telemetry.ErrorAttrs(err, "provider.request.failed")...)
	c.logger.LogAttrs(ctx, slog.LevelWarn, message, attrs...)
}

func (c *Collector) Healthz(w http.ResponseWriter, _ *http.Request) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if c.lastError != "" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("degraded: " + c.lastError + "\n"))
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}

func (c *Collector) Metrics(w http.ResponseWriter, _ *http.Request) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = fmt.Fprintf(w, "luna_devops_gateway_traffic_probe_routes %d\n", len(c.routes))
	_, _ = fmt.Fprintf(w, "luna_devops_gateway_traffic_probe_last_scrape_timestamp_seconds %d\n", c.lastScrape.Unix())
	_, _ = fmt.Fprintf(w, "luna_devops_gateway_traffic_probe_last_report_timestamp_seconds %d\n", c.lastReport.Unix())
	if c.lastError != "" {
		_, _ = fmt.Fprintln(w, "luna_devops_gateway_traffic_probe_last_error 1")
		return
	}
	_, _ = fmt.Fprintln(w, "luna_devops_gateway_traffic_probe_last_error 0")
}

func (c *Collector) scrapeAndReport(ctx context.Context) (err error) {
	ctx, end := telemetry.StartOperation(ctx, "gateway_probe", "scrape_and_report")
	defer func() { end(err) }()
	if c.reporter != nil {
		if err := c.reporter.Hello(ctx); err != nil {
			return fmt.Errorf("send probe hello: %w", err)
		}
	}
	if time.Since(c.routeLoaded) >= c.config.RouteRefreshInterval || len(c.routesSnapshot()) == 0 {
		if err := c.refreshRoutes(ctx); err != nil {
			return err
		}
	}
	routes := c.routesSnapshot()
	if len(routes) == 0 {
		c.clearError()
		return nil
	}
	counters, err := c.scrapeMetrics(ctx, routes)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Truncate(time.Minute)
	windows := c.windowsForCounters(counters, routes, now)
	reportedWindows := 0
	reportableWindows := 0
	for _, window := range windows {
		if window.ResponseBytes <= 0 {
			c.markReported(window.RouteID, counters[window.RouteID], now)
			continue
		}
		reportableWindows++
		if err := c.reporter.Report(ctx, window); err != nil {
			return err
		}
		reportedWindows++
		c.markReported(window.RouteID, counters[window.RouteID], now)
		c.logger.Info("gateway traffic window reported", "routeId", window.RouteID, "responseBytes", window.ResponseBytes, "requestCount", window.RequestCount, "periodStart", window.PeriodStart, "periodEnd", window.PeriodEnd)
	}
	c.logger.Info("gateway traffic scrape completed", "routes", len(routes), "matchedRoutes", len(counters), "windows", len(windows), "reportableWindows", reportableWindows, "reportedWindows", reportedWindows)
	c.mu.Lock()
	c.lastScrape = time.Now()
	if reportedWindows > 0 {
		c.lastReport = time.Now()
	}
	c.lastError = ""
	c.mu.Unlock()
	return nil
}

func (c *Collector) refreshRoutes(ctx context.Context) (err error) {
	ctx, end := telemetry.StartOperation(ctx, "gateway_probe", "refresh_routes")
	defer func() { end(err) }()
	routes, err := c.discoverer.ListRoutes(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.routes = routes
	c.routeLoaded = time.Now()
	c.mu.Unlock()
	c.logger.Info("gateway routes refreshed", "count", len(routes))
	return nil
}

func (c *Collector) scrapeMetrics(ctx context.Context, routes []RouteRef) (map[string]RouteCounters, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.config.TraefikMetricsURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("scrape metrics returned %d: %s", resp.StatusCode, string(body))
	}
	return ParseTraefikMetrics(resp.Body, routes)
}

func (c *Collector) routesSnapshot() []RouteRef {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]RouteRef{}, c.routes...)
}

func (c *Collector) windowsForCounters(counters map[string]RouteCounters, routes []RouteRef, windowEnd time.Time) []RouteUsageWindow {
	c.mu.RLock()
	defer c.mu.RUnlock()
	windows := make([]RouteUsageWindow, 0, len(routes))
	for _, route := range routes {
		current, ok := counters[route.ID]
		if !ok {
			continue
		}
		state, seen := c.states[route.ID]
		if !seen || !windowEnd.After(state.WindowEnd) {
			windows = append(windows, RouteUsageWindow{RouteID: route.ID, PeriodStart: windowEnd, PeriodEnd: windowEnd})
			continue
		}
		windows = append(windows, RouteUsageWindow{
			RouteID:       route.ID,
			ResponseBytes: positiveCounterDelta(current.ResponseBytes, state.Counters.ResponseBytes),
			RequestCount:  positiveCounterDelta(current.RequestCount, state.Counters.RequestCount),
			PeriodStart:   state.WindowEnd,
			PeriodEnd:     windowEnd,
		})
	}
	return windows
}

func (c *Collector) markReported(routeID string, counters RouteCounters, windowEnd time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.states[routeID] = routeState{Counters: counters, WindowEnd: windowEnd}
}

func (c *Collector) setError(err error) {
	if err == nil {
		return
	}
	c.mu.Lock()
	c.lastError = telemetry.ErrorType(err)
	c.mu.Unlock()
}

func (c *Collector) clearError() {
	c.mu.Lock()
	c.lastError = ""
	c.lastScrape = time.Now()
	c.mu.Unlock()
}

func positiveCounterDelta(current float64, previous float64) int64 {
	if current < previous {
		return int64(math.Round(current))
	}
	return int64(math.Round(current - previous))
}
