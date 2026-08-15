package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
)

const (
	browserTelemetryBodyLimit = 1024 * 1024
	browserTelemetryRateLimit = 120
)

// RelayBrowserTraces gives the authenticated web client a same-origin OTLP
// endpoint without exposing collector credentials or topology to the browser.
func (h *Handlers) RelayBrowserTraces(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	target, err := browserTraceEndpoint()
	if err != nil {
		// The browser can keep one same-origin exporter configuration in every
		// environment. Disabled server-side telemetry intentionally becomes a
		// no-op instead of generating client-side error noise.
		ctx.Status(http.StatusNoContent)
		return
	}
	allowed, err := h.rateLimiter.allow(ctx.Request.Context(), "browser_telemetry:"+user.ID, browserTelemetryRateLimit, time.Minute)
	if err != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "telemetry.rate_limit_unavailable", err.Error())
		return
	}
	if !allowed {
		writeErrorCode(ctx, http.StatusTooManyRequests, "rate_limited", "telemetry rate limit exceeded")
		return
	}
	mediaType, ok := browserTraceMediaType(ctx.GetHeader("Content-Type"))
	if !ok {
		writeErrorCode(ctx, http.StatusUnsupportedMediaType, "telemetry.invalid_content_type", "OTLP JSON or protobuf is required")
		return
	}
	body, err := io.ReadAll(io.LimitReader(ctx.Request.Body, browserTelemetryBodyLimit+1))
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "telemetry.invalid_payload", err.Error())
		return
	}
	if len(body) == 0 || len(body) > browserTelemetryBodyLimit {
		writeErrorCode(ctx, http.StatusRequestEntityTooLarge, "telemetry.payload_too_large", "telemetry payload exceeds limit")
		return
	}

	request, err := http.NewRequestWithContext(ctx.Request.Context(), http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "telemetry.relay_unavailable", err.Error())
		return
	}
	request.Header.Set("Content-Type", mediaType)
	for key, value := range otlpRelayHeaders() {
		request.Header.Set(key, value)
	}
	response, err := telemetry.InstrumentHTTPClient(&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "telemetry.relay_unavailable", err.Error())
		return
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "telemetry.relay_rejected", "collector rejected telemetry")
		return
	}
	ctx.Status(http.StatusNoContent)
}

func browserTraceMediaType(value string) (string, bool) {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
	switch mediaType {
	case "application/json", "application/x-protobuf":
		return mediaType, true
	default:
		return "", false
	}
}

func otlpRelayHeaders() map[string]string {
	value := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS"))
	if value == "" {
		value = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"))
	}
	headers := make(map[string]string)
	for _, entry := range strings.Split(value, ",") {
		key, rawValue, ok := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		rawValue = strings.TrimSpace(rawValue)
		if !ok || key == "" || strings.ContainsAny(key+rawValue, "\r\n") {
			continue
		}
		decoded, err := url.QueryUnescape(rawValue)
		if err != nil || strings.ContainsAny(decoded, "\r\n") {
			continue
		}
		headers[key] = decoded
	}
	return headers
}

func browserTraceEndpoint() (string, error) {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"))
	if endpoint == "" {
		endpoint = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
		if endpoint == "" {
			return "", errors.New("OTLP endpoint is empty")
		}
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return "", errors.New("OTLP endpoint is invalid")
		}
		parsed.Path = path.Join(parsed.Path, "/v1/traces")
		return parsed.String(), nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("OTLP traces endpoint is invalid")
	}
	return parsed.String(), nil
}
