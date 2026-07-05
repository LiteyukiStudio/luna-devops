package gatewayprobe

import "time"

type RouteRef struct {
	ID         string
	Namespace  string
	Name       string
	Hostnames  []string
	Candidates []string
}

type RouteCounters struct {
	ResponseBytes float64
	RequestCount  float64
}

type RouteUsageWindow struct {
	RouteID       string
	ResponseBytes int64
	RequestCount  int64
	PeriodStart   time.Time
	PeriodEnd     time.Time
}

type routeState struct {
	Counters  RouteCounters
	WindowEnd time.Time
}
