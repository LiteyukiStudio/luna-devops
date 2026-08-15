package volume

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestVolumeMetricDimensionsAreBounded(t *testing.T) {
	t.Parallel()

	if got := volumeMetricOperation("/user/supplied/value"); got != "unknown" {
		t.Fatalf("unrecognized operation = %q", got)
	}
	if got := volumeMetricOperation("bind"); got != "bind" {
		t.Fatalf("recognized operation = %q", got)
	}
	if got := volumeMetricSourceKind("claim-from-user"); got != "unknown" {
		t.Fatalf("unrecognized source kind = %q", got)
	}
	if got := volumeMetricSourceKind(model.ProjectVolumeSourceSnapshotRestore); got != model.ProjectVolumeSourceSnapshotRestore {
		t.Fatalf("recognized source kind = %q", got)
	}
}
