package tasks

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewVolumeTasksBuildTypedEnvelopes(t *testing.T) {
	tests := []struct {
		name        string
		taskType    string
		newTask     func() ([]byte, string, error)
		resourceRef string
		dedupeKey   string
	}{
		{
			name:     "provision",
			taskType: TypeVolumeProvision,
			newTask: func() ([]byte, string, error) {
				task, err := NewVolumeProvisionTask(VolumeProvisionPayload{VolumeID: "pvol_1", ProjectID: "prj_1", ActorID: "usr_1"})
				if err != nil {
					return nil, "", err
				}
				return task.Payload(), task.Type(), nil
			},
			resourceRef: "pvol_1",
			dedupeKey:   "volume:provision:provision:prj_1:pvol_1",
		},
		{
			name:     "import",
			taskType: TypeVolumeImport,
			newTask: func() ([]byte, string, error) {
				task, err := NewVolumeImportTask(VolumeTransferPayload{TransferID: "vtx_1", VolumeID: "pvol_1", ProjectID: "prj_1", ActorID: "usr_1"})
				if err != nil {
					return nil, "", err
				}
				return task.Payload(), task.Type(), nil
			},
			resourceRef: "vtx_1",
			dedupeKey:   "volume:import:prj_1:vtx_1",
		},
		{
			name:     "export",
			taskType: TypeVolumeExport,
			newTask: func() ([]byte, string, error) {
				task, err := NewVolumeExportTask(VolumeTransferPayload{TransferID: "vtx_2", VolumeID: "pvol_1", ProjectID: "prj_1", ActorID: "usr_1"})
				if err != nil {
					return nil, "", err
				}
				return task.Payload(), task.Type(), nil
			},
			resourceRef: "vtx_2",
			dedupeKey:   "volume:export:prj_1:vtx_2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, taskType, err := tt.newTask()
			if err != nil {
				t.Fatalf("new task: %v", err)
			}
			if taskType != tt.taskType {
				t.Fatalf("task type = %q, want %q", taskType, tt.taskType)
			}
			var document struct {
				Envelope TaskEnvelope `json:"envelope"`
			}
			if err := json.Unmarshal(payload, &document); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			if document.Envelope.ResourceRef != tt.resourceRef || document.Envelope.DedupeKey != tt.dedupeKey {
				t.Fatalf("envelope = %#v", document.Envelope)
			}
		})
	}
}

func TestNewVolumeTasksRejectMissingIdentity(t *testing.T) {
	if _, err := NewVolumeProvisionTask(VolumeProvisionPayload{ProjectID: "prj_1"}); err == nil {
		t.Fatal("expected missing volume id error")
	}
	if _, err := NewVolumeDeleteTask(VolumeDeletePayload{VolumeID: "pvol_1"}); err == nil {
		t.Fatal("expected missing project id error")
	}
	if _, err := NewVolumeImportTask(VolumeTransferPayload{VolumeID: "pvol_1", ProjectID: "prj_1"}); err == nil {
		t.Fatal("expected missing transfer id error")
	}
	if _, err := NewVolumeProvisionTask(VolumeProvisionPayload{VolumeID: "pvol_1", ProjectID: "prj_1", Operation: "shrink"}); err == nil {
		t.Fatal("expected invalid provision operation error")
	}
}

func TestNewVolumeProvisionTaskDistinguishesExpansion(t *testing.T) {
	task, err := NewVolumeProvisionTask(VolumeProvisionPayload{VolumeID: "pvol_1", ProjectID: "prj_1", Operation: VolumeOperationExpand})
	if err != nil {
		t.Fatalf("NewVolumeProvisionTask returned error: %v", err)
	}
	var payload VolumeProvisionPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Operation != VolumeOperationExpand || payload.Envelope.DedupeKey != "volume:provision:expand:prj_1:pvol_1" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestVolumeMaintenanceTasksUseStableSystemDedupeKeys(t *testing.T) {
	reconcile, err := NewVolumeReconcileTask(VolumeReconcilePayload{})
	if err != nil {
		t.Fatalf("new reconcile task: %v", err)
	}
	cleanup, err := NewVolumeTransferCleanupTask(VolumeTransferCleanupPayload{})
	if err != nil {
		t.Fatalf("new cleanup task: %v", err)
	}

	for _, test := range []struct {
		payload []byte
		want    string
	}{{reconcile.Payload(), "volume:reconcile:system:stale-volumes"}, {cleanup.Payload(), "volume-transfer:cleanup:system:expired-transfers"}} {
		var document struct {
			Envelope TaskEnvelope `json:"envelope"`
		}
		if err := json.Unmarshal(test.payload, &document); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if document.Envelope.DedupeKey != test.want {
			t.Fatalf("dedupe key = %q, want %q", document.Envelope.DedupeKey, test.want)
		}
	}
}

func TestVolumeTaskPoliciesAreBounded(t *testing.T) {
	for _, taskType := range []string{TypeVolumeProvision, TypeVolumeImport, TypeVolumeExport, TypeVolumeDelete} {
		policy := PolicyForType(taskType)
		if policy.Queue != QueueDeploy || policy.Timeout <= 0 || policy.Unique <= 0 || policy.Retention != 24*time.Hour {
			t.Fatalf("policy for %s = %#v", taskType, policy)
		}
	}
	for _, taskType := range []string{TypeVolumeReconcile, TypeVolumeTransferCleanup} {
		policy := PolicyForType(taskType)
		if policy.Queue != QueueLight || policy.Timeout <= 0 || policy.Unique <= 0 {
			t.Fatalf("policy for %s = %#v", taskType, policy)
		}
	}
}
