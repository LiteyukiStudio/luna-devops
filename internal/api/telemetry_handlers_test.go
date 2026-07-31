package api

import "testing"

func TestBrowserTraceEndpointUsesGenericOTLPBase(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318/otel")
	endpoint, err := browserTraceEndpoint()
	if err != nil {
		t.Fatalf("resolve browser trace endpoint: %v", err)
	}
	if endpoint != "http://collector:4318/otel/v1/traces" {
		t.Fatalf("unexpected endpoint %q", endpoint)
	}
}

func TestOTLPRelayHeadersDoNotForwardMalformedValues(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS", "api-key=secret%20value,bad,nope=x%0D%0Ay")
	headers := otlpRelayHeaders()
	if headers["api-key"] != "secret value" {
		t.Fatalf("expected decoded API key, got %q", headers["api-key"])
	}
	if _, exists := headers["bad"]; exists {
		t.Fatal("malformed header was forwarded")
	}
	if _, exists := headers["nope"]; exists {
		t.Fatal("CRLF header was forwarded")
	}
}
