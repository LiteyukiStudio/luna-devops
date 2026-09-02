package api

import (
	"context"
	"net/http/httptest"

	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
)

func volumeTestContext(method, target string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, nil)
	return ctx, recorder
}

// volumeTaskEnqueuerStub remains in the root integration test package because
// AI end-to-end tests exercise the root volume dispatcher directly.
type volumeTaskEnqueuerStub struct {
	provision    tasks.VolumeProvisionPayload
	cleanup      tasks.VolumeTransferCleanupPayload
	contextValue string
}

func (stub *volumeTaskEnqueuerStub) EnqueueVolumeProvision(ctx context.Context, payload tasks.VolumeProvisionPayload) (*asynq.TaskInfo, error) {
	stub.provision = payload
	return &asynq.TaskInfo{}, nil
}

func (*volumeTaskEnqueuerStub) EnqueueVolumeImport(context.Context, tasks.VolumeTransferPayload) (*asynq.TaskInfo, error) {
	return &asynq.TaskInfo{}, nil
}

func (*volumeTaskEnqueuerStub) EnqueueVolumeExport(context.Context, tasks.VolumeTransferPayload) (*asynq.TaskInfo, error) {
	return &asynq.TaskInfo{}, nil
}

func (stub *volumeTaskEnqueuerStub) EnqueueVolumeTransferCleanup(_ context.Context, payload tasks.VolumeTransferCleanupPayload) (*asynq.TaskInfo, error) {
	stub.cleanup = payload
	return &asynq.TaskInfo{}, nil
}

func (*volumeTaskEnqueuerStub) EnqueueVolumeDelete(context.Context, tasks.VolumeDeletePayload) (*asynq.TaskInfo, error) {
	return &asynq.TaskInfo{}, nil
}
