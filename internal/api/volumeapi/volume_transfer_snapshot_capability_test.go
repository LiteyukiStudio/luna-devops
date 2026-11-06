package volumeapi

import (
	"testing"

	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

func TestStorageClassSnapshotSupportedMatchesNamedOrDefaultClass(t *testing.T) {
	classes := []kubeprovider.VolumeStorageClass{
		{Name: "fast", SnapshotSupported: true},
		{Name: "standard", IsDefault: true},
	}
	for _, test := range []struct {
		className string
		want      bool
	}{
		{className: "fast", want: true},
		{className: "standard", want: false},
		{className: "", want: false},
		{className: "missing", want: false},
	} {
		if got := storageClassSnapshotSupported(classes, test.className); got != test.want {
			t.Fatalf("storageClassSnapshotSupported(%q) = %t, want %t", test.className, got, test.want)
		}
	}
	classes[1].SnapshotSupported = true
	if !storageClassSnapshotSupported(classes, "") {
		t.Fatal("blank class name must resolve to the default StorageClass")
	}
}
