package kubeproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	storagev1 "k8s.io/api/storage/v1"
)

type staticLocalSource []storagev1.StorageClass

func (source staticLocalSource) StorageClasses(context.Context, AccessContext) ([]storagev1.StorageClass, error) {
	return append([]storagev1.StorageClass(nil), source...), nil
}

func TestLocalNamespaceReturnsOnlyBoundNamespace(t *testing.T) {
	handler := LocalResourceHandler{}
	request := httptest.NewRequest(http.MethodGet, "https://gateway.example/api/v1/namespaces", nil)
	writer := httptest.NewRecorder()
	info := RequestInfo{Verb: "list", APIVersion: "v1", Resource: "namespaces", IsResourceRequest: true, IsCollection: true}
	if err := handler.Serve(t.Context(), writer, request, baseAccess(), info); err != nil {
		t.Fatal(err)
	}
	if writer.Code != http.StatusOK || !containsBytes(writer.Body.Bytes(), []byte(`"name":"project-a"`)) {
		t.Fatalf("unexpected response: %d %s", writer.Code, writer.Body.String())
	}
}

func TestNegotiationRejectsUnsupportedOnlyAccept(t *testing.T) {
	if _, err := NegotiateRepresentation("text/html"); err == nil {
		t.Fatal("unsupported Accept must return 406")
	}
}

func TestNegotiationUsesQualityAndValidatesMetaVersion(t *testing.T) {
	representation, err := NegotiateRepresentation("application/yaml;q=0.4, application/json;q=0.9")
	if err != nil {
		t.Fatal(err)
	}
	if representation.MediaType != "application/json" {
		t.Fatalf("quality negotiation selected %q", representation.MediaType)
	}
	if _, err := NegotiateRepresentation("application/json;as=Table;g=other.io;v=v1"); err == nil {
		t.Fatal("unsupported Table group must return 406")
	}
}

func containsBytes(value, substring []byte) bool {
	for index := 0; index+len(substring) <= len(value); index++ {
		match := true
		for offset := range substring {
			if value[index+offset] != substring[offset] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
