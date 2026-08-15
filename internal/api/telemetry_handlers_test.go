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
			got, allowed := browserTraceMediaType(test.input)
			if got != test.want || allowed != test.allowed {
				t.Fatalf("browserTraceMediaType(%q) = (%q, %v), want (%q, %v)", test.input, got, allowed, test.want, test.allowed)
			}
		})
	}
}
