package gatewayprobe

import (
	"testing"
	"time"
)

func TestCollectorBuildsUsageWindowFromCounterDelta(t *testing.T) {
	collector := NewCollector(Config{}, nil, nil, nil)
	route := RouteRef{ID: "gwr_1"}
	start := time.Date(2026, 7, 6, 1, 2, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	collector.markReported(route.ID, RouteCounters{ResponseBytes: 1000, RequestCount: 10}, start)
	windows := collector.windowsForCounters(map[string]RouteCounters{
		route.ID: {ResponseBytes: 2500, RequestCount: 25},
	}, []RouteRef{route}, end)
	if len(windows) != 1 {
		t.Fatalf("windows = %#v", windows)
	}
	window := windows[0]
	if window.ResponseBytes != 1500 || window.RequestCount != 15 || !window.PeriodStart.Equal(start) || !window.PeriodEnd.Equal(end) {
		t.Fatalf("window = %#v", window)
	}
}
