package main

import (
	"errors"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/volumemigration"
)

func TestParseCommandOptionsDefaultsToDryRun(t *testing.T) {
	options, err := parseCommandOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	if options.Apply || options.PageSize != volumemigration.DefaultPageSize || options.ProjectID != "" || options.ObservationTimeout != 8*time.Second {
		t.Fatalf("default options = %+v", options)
	}
}

func TestParseCommandOptionsAcceptsScopedApply(t *testing.T) {
	options, err := parseCommandOptions([]string{"--apply", "--page-size=20", "--project-id= prj_demo ", "--observation-timeout=3s"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.Apply || options.PageSize != 20 || options.ProjectID != "prj_demo" || options.ObservationTimeout != 3*time.Second {
		t.Fatalf("parsed options = %+v", options)
	}
}

func TestParseCommandOptionsRejectsUnsafePagination(t *testing.T) {
	for _, args := range [][]string{{"--page-size=0"}, {"--page-size=101"}, {"unexpected"}} {
		if _, err := parseCommandOptions(args); !errors.Is(err, volumemigration.ErrInvalidOptions) {
			t.Fatalf("parseCommandOptions(%v) error = %v", args, err)
		}
	}
}
