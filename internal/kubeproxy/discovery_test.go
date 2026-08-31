package kubeproxy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRewriteOpenAPIV3PrefixesServerRelativeURLs(t *testing.T) {
	input := []byte(`{"paths":{"apis/apps/v1":{"serverRelativeURL":"/openapi/v3/apis/apps/v1?hash=x"}}}`)
	output, err := RewriteOpenAPIV3(input, "/kube/v1/bindings/b1")
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(output, &value); err != nil {
		t.Fatal(err)
	}
	got := value["paths"].(map[string]any)["apis/apps/v1"].(map[string]any)["serverRelativeURL"]
	if got != "/kube/v1/bindings/b1/openapi/v3/apis/apps/v1?hash=x" {
		t.Fatalf("unexpected URL: %v", got)
	}
}

func TestRewriteOpenAPIV3RemovesAbsoluteUpstreamAuthority(t *testing.T) {
	input := []byte(`{"paths":{"apis/apps/v1":{"serverRelativeURL":"https://token@upstream.internal/openapi/v3/apis/apps/v1?hash=x"}}}`)
	output, err := RewriteOpenAPIV3(input, "/kube/v1/bindings/b1")
	if err != nil {
		t.Fatal(err)
	}
	text := string(output)
	if strings.Contains(text, "upstream.internal") || strings.Contains(text, "token@") || !strings.Contains(text, "/kube/v1/bindings/b1/openapi/v3/apis/apps/v1") {
		t.Fatalf("absolute authority was not safely rewritten: %s", text)
	}
}

func TestDiscoveryTransformerRewritesGzipOpenAPI(t *testing.T) {
	compressed := &bytes.Buffer{}
	writer := gzip.NewWriter(compressed)
	_, _ = writer.Write([]byte(`{"paths":{"apis/apps/v1":{"serverRelativeURL":"/openapi/v3/apis/apps/v1"}}}`))
	_ = writer.Close()
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "Content-Encoding": []string{"gzip"}, "ETag": []string{"upstream"}},
		Body:       io.NopCloser(bytes.NewReader(compressed.Bytes())),
	}
	err := (DiscoveryTransformer{KubePrefix: "/kube/v1/bindings/b1", RequestPath: "/openapi/v3"}).Transform(t.Context(), response)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	decompressed, _ := io.ReadAll(reader)
	_ = reader.Close()
	if !strings.Contains(string(decompressed), "/kube/v1/bindings/b1/openapi/v3/apis/apps/v1") || response.Header.Get("ETag") != "" {
		t.Fatalf("gzip discovery response was not safely rewritten: %s %#v", decompressed, response.Header)
	}
}

func TestRewriteLocationRemovesUpstreamAuthority(t *testing.T) {
	got := RewriteLocation("https://upstream.internal/api/v1/namespaces/a/pods/p", "/kube/v1/bindings/b1")
	if got != "/kube/v1/bindings/b1/api/v1/namespaces/a/pods/p" {
		t.Fatalf("unexpected location: %s", got)
	}
}

func TestDiscoveryTransformerClearsGzipRootDiscoveryAddresses(t *testing.T) {
	compressed := &bytes.Buffer{}
	writer := gzip.NewWriter(compressed)
	_, _ = writer.Write([]byte(`{"kind":"APIGroupList","apiVersion":"v1","groups":[{"name":"apps","serverAddressByClientCIDRs":[{"clientCIDR":"0.0.0.0/0","serverAddress":"https://upstream.internal:6443"}]}]}`))
	_ = writer.Close()
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":     []string{"application/json"},
			"Content-Encoding": []string{"gzip"},
			"ETag":             []string{"upstream"},
		},
		Body: io.NopCloser(bytes.NewReader(compressed.Bytes())),
	}
	if err := (DiscoveryTransformer{KubePrefix: "/kube/v1/bindings/b1", RequestPath: "/apis"}).Transform(t.Context(), response); err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	decompressed, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(decompressed), "upstream.internal") || !strings.Contains(string(decompressed), `"serverAddressByClientCIDRs":[]`) {
		t.Fatalf("root discovery leaked an upstream address: %s", decompressed)
	}
	if response.Header.Get("Content-Type") != "application/json" || response.Header.Get("Content-Encoding") != "gzip" || response.Header.Get("ETag") != "" {
		t.Fatalf("root discovery representation headers were not preserved safely: %#v", response.Header)
	}
}
