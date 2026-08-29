package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
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
	target, err := browserTraceEndpoint(h.config)
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
	for key, value := range otlpRelayHeaders(h.config) {
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

func otlpRelayHeaders(configs ...Config) map[string]string {
	configured := configuredOrLoaded(configs).BrowserTraceHeaders
	headers := make(map[string]string, len(configured))
	for key, value := range configured {
		headers[key] = value
	}
	return headers
}

func browserTraceEndpoint(configs ...Config) (string, error) {
	endpoint := configuredOrLoaded(configs).BrowserTraceEndpoint
	if endpoint == "" {
		return "", errors.New("OTLP endpoint is empty")
	}
	return endpoint, nil
}
