package main

import (
	"reflect"
	"testing"
)

func TestParseOptionsRequiresSensitiveMetadataAcknowledgement(t *testing.T) {
	if _, err := parseOptions(nil); err == nil {
		t.Fatal("parseOptions() accepted an unacknowledged metadata scan")
	}
	options, err := parseOptions([]string{"--acknowledge-sensitive-metadata", "--project-id", " project-1 "})
	if err != nil {
		t.Fatal(err)
	}
	if options.ProjectID != "project-1" || options.PageSize != defaultPageSize {
		t.Fatalf("options = %#v", options)
	}
}

func TestInspectRowReturnsOnlyResourceMetadataAndKeys(t *testing.T) {
	finding, ok, err := inspectRow("deployment_target", "target-1", "project-1", `{"TOKEN":"must-not-leak","LOG_LEVEL":"debug"}`)
	if err != nil || !ok {
		t.Fatalf("inspectRow() = %#v, %v, %v", finding, ok, err)
	}
	if !reflect.DeepEqual(finding.Keys, []string{"TOKEN"}) {
		t.Fatalf("keys = %#v", finding.Keys)
	}
	if finding.ResourceID != "target-1" || finding.ProjectID != "project-1" {
		t.Fatalf("finding = %#v", finding)
	}
}
