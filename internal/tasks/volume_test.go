package tasks

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

func TestNewVolumeTasksBuildTypedPayloads(t *testing.T) {
	tests := []struct {
		name     string
		taskType string
		newTask  func() ([]byte, string, error)
		want     map[string]string
	}{
		{
			name:     "provision",
			taskType: TypeVolumeProvision,
			newTask: func() ([]byte, string, error) {
				task, err := NewVolumeProvisionTask(VolumeProvisionPayload{VolumeID: "pvol_1", ProjectID: "prj_1"})
				if err != nil {
					return nil, "", err
				}
				return task.Payload(), task.Type(), nil
			},
			want: map[string]string{"volumeId": "pvol_1", "projectId": "prj_1", "operation": VolumeOperationProvision},
		},
		{
			name:     "import",
			taskType: TypeVolumeImport,
			newTask: func() ([]byte, string, error) {
				task, err := NewVolumeImportTask(VolumeTransferPayload{TransferID: "vtx_1", VolumeID: "pvol_1", ProjectID: "prj_1"})
				if err != nil {
					return nil, "", err
				}
				return task.Payload(), task.Type(), nil
			},
			want: map[string]string{"transferId": "vtx_1", "volumeId": "pvol_1", "projectId": "prj_1"},
		},
		{
			name:     "export",
			taskType: TypeVolumeExport,
			newTask: func() ([]byte, string, error) {
				task, err := NewVolumeExportTask(VolumeTransferPayload{TransferID: "vtx_2", VolumeID: "pvol_1", ProjectID: "prj_1"})
				if err != nil {
					return nil, "", err
				}
				return task.Payload(), task.Type(), nil
			},
			want: map[string]string{"transferId": "vtx_2", "volumeId": "pvol_1", "projectId": "prj_1"},
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
			var document map[string]any
			if err := json.Unmarshal(payload, &document); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			for key, want := range tt.want {
				if document[key] != want {
					t.Fatalf("payload[%s] = %#v, want %q", key, document[key], want)
				}
			}
			if _, ok := document["actorId"]; ok {
				t.Fatalf("task payload must not include mutable actor identity: %s", payload)
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
	if payload.Operation != VolumeOperationExpand {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestVolumeMaintenanceTaskPayloadsAreStable(t *testing.T) {
	for _, newTask := range []func() (*asynq.Task, error){
		func() (*asynq.Task, error) { return NewVolumeReconcileTask(VolumeReconcilePayload{}) },
		func() (*asynq.Task, error) { return NewVolumeTransferCleanupTask(VolumeTransferCleanupPayload{}) },
	} {
		first, err := newTask()
		if err != nil {
			t.Fatalf("new first maintenance task: %v", err)
		}
		second, err := newTask()
		if err != nil {
			t.Fatalf("new second maintenance task: %v", err)
		}
		if string(first.Payload()) != string(second.Payload()) {
			t.Fatalf("maintenance payloads differ: %s / %s", first.Payload(), second.Payload())
		}
	}
}

func TestVolumeTaskPoliciesAreBounded(t *testing.T) {
	for _, taskType := range []string{TypeVolumeProvision, TypeVolumeImport, TypeVolumeExport, TypeVolumeDelete} {
		policy := PolicyForType(taskType)
		if policy.Queue != QueueDeploy || policy.Timeout <= 0 || policy.Unique != 24*time.Hour || policy.Retention != 24*time.Hour {
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
