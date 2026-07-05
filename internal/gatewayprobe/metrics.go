package gatewayprobe

import (
	"io"
	"math"
	"strings"
	"unicode"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

func ParseTraefikMetrics(reader io.Reader, routes []RouteRef) (map[string]RouteCounters, error) {
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(reader)
	if err != nil {
		return nil, err
	}
	return CountersFromMetricFamilies(families, routes), nil
}

func CountersFromMetricFamilies(families map[string]*dto.MetricFamily, routes []RouteRef) map[string]RouteCounters {
	output := map[string]RouteCounters{}
	for name, family := range families {
		if family == nil {
			continue
		}
		kind := metricKind(name)
		if kind == "" {
			continue
		}
		for _, metric := range family.Metric {
			value, ok := metricCounterValue(metric)
			if !ok {
				continue
			}
			routeID := routeIDForMetric(metric, routes)
			if routeID == "" {
				continue
			}
			counters := output[routeID]
			if kind == "response_bytes" {
				counters.ResponseBytes += value
			}
			if kind == "requests" {
				counters.RequestCount += value
			}
			output[routeID] = counters
		}
	}
	return output
}

func metricKind(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if strings.Contains(name, "response") && strings.Contains(name, "bytes") && strings.HasSuffix(name, "_total") {
		return "response_bytes"
	}
	if strings.HasSuffix(name, "_requests_total") && !strings.Contains(name, "bytes") && !strings.Contains(name, "duration") {
		return "requests"
	}
	return ""
}

func metricCounterValue(metric *dto.Metric) (float64, bool) {
	if metric == nil || metric.Counter == nil || metric.Counter.Value == nil {
		return 0, false
	}
	value := metric.Counter.GetValue()
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, false
	}
	return value, true
}

func routeIDForMetric(metric *dto.Metric, routes []RouteRef) string {
	labels := normalizedMetricLabelValues(metric)
	bestRouteID := ""
	bestScore := 0
	for _, route := range routes {
		for _, candidate := range route.Candidates {
			candidate = normalizeMetricToken(candidate)
			if candidate == "" {
				continue
			}
			for _, labelValue := range labels {
				if !strings.Contains(labelValue, candidate) {
					continue
				}
				if len(candidate) > bestScore {
					bestRouteID = route.ID
					bestScore = len(candidate)
				}
			}
		}
	}
	return bestRouteID
}

func normalizedMetricLabelValues(metric *dto.Metric) []string {
	if metric == nil {
		return nil
	}
	values := make([]string, 0, len(metric.Label))
	for _, label := range metric.Label {
		if label == nil || label.Value == nil {
			continue
		}
		values = append(values, normalizeMetricToken(label.GetValue()))
	}
	return values
}

func normalizeMetricToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, item := range value {
		if unicode.IsLetter(item) || unicode.IsDigit(item) {
			builder.WriteRune(item)
		}
	}
	return builder.String()
}
