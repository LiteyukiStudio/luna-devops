package api

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/platformapi"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
)

func TestBrowserTraceEndpointUsesGenericOTLPBase(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318/otel")
	endpoint, err := platformapi.BrowserTraceEndpoint(mustTestConfig(t))
	if err != nil {
		t.Fatalf("resolve browser trace endpoint: %v", err)
	}
	if endpoint != "http://collector:4318/otel/v1/traces" {
		t.Fatalf("unexpected endpoint %q", endpoint)
	}
}

func TestOTLPRelayHeadersRejectMalformedValues(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS", "api-key=secret%20value,bad,nope=x%0D%0Ay")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("malformed OTLP relay headers were accepted")
	}
}

func TestBrowserTraceMediaTypeAcceptsStandardOTLPHTTPEncodings(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		allowed bool
	}{
		{name: "json", input: "application/json; charset=utf-8", want: "application/json", allowed: true},
		{name: "protobuf", input: "application/x-protobuf", want: "application/x-protobuf", allowed: true},
		{name: "case insensitive", input: " Application/JSON ", want: "application/json", allowed: true},
		{name: "plain text", input: "text/plain", allowed: false},
		{name: "missing", input: "", allowed: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, allowed := platformapi.BrowserTraceMediaType(test.input)
			if got != test.want || allowed != test.allowed {
				t.Fatalf("browserTraceMediaType(%q) = (%q, %v), want (%q, %v)", test.input, got, allowed, test.want, test.allowed)
			}
		})
	}
}

func TestRelayBrowserTracesForwardsOTLPProtobuf(t *testing.T) {
	payload := []byte{0x0a, 0x03, 0x01, 0x02, 0x03}
	type collectorRequest struct {
		body          []byte
		contentType   string
		authorization string
	}
	received := make(chan collectorRequest, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- collectorRequest{
			body:          body,
			contentType:   request.Header.Get("Content-Type"),
			authorization: request.Header.Get("Authorization"),
		}
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(collector.Close)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", collector.URL+"/v1/traces")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS", "Authorization=Bearer%20relay-secret")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "")

	recorder := httptest.NewRecorder()
	router := newBrowserTraceRelayTestRouter(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/telemetry/v1/traces", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/x-protobuf; proto=otlp")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("relay status = %d, want %d; body = %s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
	var forwarded collectorRequest
	select {
	case forwarded = <-received:
	case <-time.After(time.Second):
		t.Fatal("collector did not receive the relayed trace")
	}
	if !bytes.Equal(forwarded.body, payload) {
		t.Fatalf("collector body = %v, want %v", forwarded.body, payload)
	}
	if forwarded.contentType != "application/x-protobuf" {
		t.Fatalf("collector Content-Type = %q, want application/x-protobuf", forwarded.contentType)
	}
	if forwarded.authorization != "Bearer relay-secret" {
		t.Fatalf("collector Authorization = %q, want configured relay credential", forwarded.authorization)
	}
}

func TestRelayBrowserTracesRejectsUnsupportedMediaTypeBeforeCollector(t *testing.T) {
	var collectorCalls atomic.Int32
	collector := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		collectorCalls.Add(1)
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(collector.Close)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", collector.URL+"/v1/traces")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	recorder := httptest.NewRecorder()
	router := newBrowserTraceRelayTestRouter(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/telemetry/v1/traces", bytes.NewReader([]byte("not OTLP")))
	request.Header.Set("Content-Type", "text/plain")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("relay status = %d, want %d; body = %s", recorder.Code, http.StatusUnsupportedMediaType, recorder.Body.String())
	}
	if calls := collectorCalls.Load(); calls != 0 {
		t.Fatalf("collector received %d requests for an unsupported media type, want 0", calls)
	}
}

func newBrowserTraceRelayTestRouter(t *testing.T) http.Handler {
	t.Helper()
	redisServer := miniredis.RunT(t)
	cfg := mustTestConfig(t)
	handlers := &Handlers{config: cfg, rateLimiter: newRateLimiter(redisServer.Addr())}
	handlers.domains = newDomainHandlers(handlers)
	t.Cleanup(func() {
		_ = handlers.rateLimiter.redis.Close()
	})
	router := gin.New()
	router.Use(func(ctx *gin.Context) {
		ctx.Set(currentUserContextKey, model.User{ID: "usr_browser_telemetry_test"})
		ctx.Next()
	})
	router.POST("/api/v1/telemetry/v1/traces", handlers.domains.platform.RelayBrowserTraces)
	return router
}
