package kubeproxy

import (
	"reflect"
	"strings"
	"testing"
)

func TestAuditEventTypeCannotAcceptSensitivePayloads(t *testing.T) {
	typeValue := reflect.TypeOf(AuditEvent{})
	for index := 0; index < typeValue.NumField(); index++ {
		name := strings.ToLower(typeValue.Field(index).Name)
		for _, forbidden := range []string{"body", "header", "query", "command", "token", "secret", "cookie"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("AuditEvent exposes sensitive field %s", typeValue.Field(index).Name)
			}
		}
	}
}

func TestAuditRequirementCoversMutationAndStreams(t *testing.T) {
	if !ShouldPersistAudit(RequestInfo{Verb: "patch"}, Decision{}) {
		t.Fatal("mutation must be audited")
	}
	if !ShouldPersistAudit(RequestInfo{Verb: "get", Transport: TransportLogs}, Decision{}) {
		t.Fatal("logs must be audited")
	}
	if ShouldPersistAudit(RequestInfo{Verb: "get"}, Decision{}) {
		t.Fatal("ordinary non-sensitive get does not require persistent audit")
	}
}
