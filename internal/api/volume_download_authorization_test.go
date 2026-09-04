package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type volumeDownloadAuthorizationState struct {
	role              atomic.Value
	memberMissing     atomic.Bool
	transferDirection atomic.Value
	transferState     atomic.Value
	transferActorID   atomic.Value
	personalTokenDead atomic.Bool
}

func newVolumeDownloadAuthorizationTestHandlers(t *testing.T) (*Handlers, *volumeDownloadAuthorizationState) {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test password=test dbname=test port=1 sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}
	state := &volumeDownloadAuthorizationState{}
	state.role.Store(authz.ProjectRoleOwner)
	state.transferDirection.Store(model.VolumeTransferDirectionExport)
	state.transferState.Store(model.VolumeTransferStateStreaming)
	state.transferActorID.Store("usr_download")
	if err := db.Callback().Query().Replace("gorm:query", func(query *gorm.DB) {
		now := time.Now()
		switch destination := query.Statement.Dest.(type) {
		case *model.UserSession:
			*destination = model.UserSession{ID: "ses_download", UserID: "usr_download", ExpiresAt: now.Add(time.Hour)}
		case *model.User:
			*destination = model.User{ID: "usr_download", Role: authz.PlatformRoleUser}
		case *model.AccessToken:
			*destination = model.AccessToken{ID: "tok_download", UserID: "usr_download", Source: "personal"}
			if state.personalTokenDead.Load() {
				now := time.Now()
				destination.RevokedAt = &now
			}
		case *model.Project:
			*destination = model.Project{ID: "prj_download", DeleteStatus: "active"}
		case *model.ProjectMember:
			if state.memberMissing.Load() {
				query.AddError(gorm.ErrRecordNotFound)
				query.RowsAffected = 0
				return
			}
			*destination = model.ProjectMember{ProjectID: "prj_download", UserID: "usr_download", Role: state.role.Load().(string)}
		case *model.VolumeTransfer:
			*destination = model.VolumeTransfer{
				ID: "vtx_download", ProjectID: "prj_download", Direction: state.transferDirection.Load().(string),
				State: state.transferState.Load().(string), ActorID: state.transferActorID.Load().(string), ExpiresAt: now.Add(time.Hour),
			}
		default:
			query.AddError(errors.New("unexpected authorization query"))
		}
		query.RowsAffected = 1
	}); err != nil {
		t.Fatalf("replace query callback: %v", err)
	}
	handlers := &Handlers{db: db}
	handlers.domains = newDomainHandlers(handlers)
	return handlers, state
}

func TestContinuousAuthorizationPersonalTokenRevocationCancelsReadBarrier(t *testing.T) {
	handlers, state := newVolumeDownloadAuthorizationTestHandlers(t)
	binding := continuousAuthorizationBindingForAccessToken("usr_download", model.AccessToken{
		ID: "tok_download", UserID: "usr_download", Source: "personal",
	})
	streamCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	revoked, active := handlers.monitorContinuousAuthorizationWithInterval(
		streamCtx,
		binding,
		func(context.Context, model.User) bool { return true },
		cancel,
		5*time.Millisecond,
	)
	if !active {
		t.Fatal("initial personal-token authorization unexpectedly failed")
	}
	body := newAuthorizationBlockingReadCloser()
	var output bytes.Buffer
	copyDone := make(chan error, 1)
	go func() { copyDone <- copyVolumeDownloadBody(streamCtx, &output, body, nil) }()
	select {
	case <-body.started:
	case <-time.After(time.Second):
		t.Fatal("read barrier did not start")
	}
	state.personalTokenDead.Store(true)
	select {
	case <-revoked:
	case <-time.After(time.Second):
		t.Fatal("personal-token revocation did not cancel the read barrier")
	}
	select {
	case err := <-copyDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("read barrier error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("read barrier continued after personal-token revocation")
	}
	if !body.closed.Load() || output.Len() != 0 {
		t.Fatalf("revoked read barrier closed=%v output=%d", body.closed.Load(), output.Len())
	}
}

type blockingVolumeImportContentService struct {
	started   chan struct{}
	committed atomic.Bool
	once      sync.Once
}

func newBlockingVolumeImportContentService() *blockingVolumeImportContentService {
	return &blockingVolumeImportContentService{started: make(chan struct{})}
}

func (*blockingVolumeImportContentService) CreateImport(context.Context, model.User, model.Project, volumeImportCreateInput, string) (model.ProjectVolume, model.VolumeTransfer, error) {
	panic("unexpected CreateImport")
}

func (service *blockingVolumeImportContentService) StreamImport(ctx context.Context, _, _ string, _ model.User, body io.Reader, _ int64) (model.VolumeTransfer, error) {
	service.once.Do(func() { close(service.started) })
	_, err := body.Read(make([]byte, 1))
	if err != nil {
		return model.VolumeTransfer{}, err
	}
	if err := ctx.Err(); err != nil {
		return model.VolumeTransfer{}, err
	}
	service.committed.Store(true)
	return model.VolumeTransfer{ID: "vtx_download", Format: "tar.gz"}, nil
}

func (*blockingVolumeImportContentService) CreateExport(context.Context, model.User, model.Project, string, volumeExportCreateInput, string) (model.VolumeTransfer, error) {
	panic("unexpected CreateExport")
}

func (*blockingVolumeImportContentService) RetryTransfer(context.Context, model.User, model.Project, model.VolumeTransfer, string) (model.VolumeTransfer, error) {
	panic("unexpected RetryTransfer")
}

func (*blockingVolumeImportContentService) AuthorizeDownload(context.Context, model.User, model.Project, model.VolumeTransfer, volumeDownloadBinding) (volumeDownloadAuthorizationResponse, error) {
	panic("unexpected AuthorizeDownload")
}

func (*blockingVolumeImportContentService) OpenDownload(context.Context, model.User, model.Project, model.VolumeTransfer, string, volumeDownloadBinding) (volumeDownload, error) {
	panic("unexpected OpenDownload")
}

func (*blockingVolumeImportContentService) OpenManifest(context.Context, model.User, model.Project, model.VolumeTransfer, string, volumeDownloadBinding) (volumeDownload, error) {
	panic("unexpected OpenManifest")
}

type blockingImportRequestBody struct {
	started   chan struct{}
	release   chan struct{}
	readOnce  sync.Once
	closeOnce sync.Once
	closed    atomic.Bool
}

func newBlockingImportRequestBody() *blockingImportRequestBody {
	return &blockingImportRequestBody{started: make(chan struct{}), release: make(chan struct{})}
}

func (body *blockingImportRequestBody) Read([]byte) (int, error) {
	body.readOnce.Do(func() { close(body.started) })
	<-body.release
	return 0, context.Canceled
}

func (body *blockingImportRequestBody) Close() error {
	body.closeOnce.Do(func() {
		body.closed.Store(true)
		close(body.release)
	})
	return nil
}

func TestVolumeImportManagerDowngradeInterruptsBodyWithoutCommit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handlers, state := newVolumeDownloadAuthorizationTestHandlers(t)
	state.role.Store(authz.ProjectRoleAdmin)
	state.transferDirection.Store(model.VolumeTransferDirectionImport)
	state.transferState.Store(model.VolumeTransferStateReady)
	state.transferActorID.Store("usr_other_actor")
	handlers.continuousAuthorizationInterval = 5 * time.Millisecond
	content := newBlockingVolumeImportContentService()
	handlers.volumeContent = content
	handlers.domains = newDomainHandlers(handlers)
	body := newBlockingImportRequestBody()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/v1/projects/prj_download/volume-imports/vtx_download/content", body)
	ctx.Request.ContentLength = 1
	ctx.Request.Header.Set("Content-Type", "application/octet-stream")
	ctx.Request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-test"})
	ctx.Params = gin.Params{{Key: "projectId", Value: "prj_download"}, {Key: "transferId", Value: "vtx_download"}}
	ctx.Set(currentUserContextKey, model.User{ID: "usr_download", Role: authz.PlatformRoleUser})

	done := make(chan struct{})
	go func() {
		handlers.domains.volume.UploadVolumeImportContent(ctx)
		close(done)
	}()
	select {
	case <-content.started:
	case <-time.After(time.Second):
		t.Fatal("volume import service did not start")
	}
	select {
	case <-body.started:
	case <-time.After(time.Second):
		t.Fatal("volume import request body did not block")
	}
	state.role.Store(authz.ProjectRoleDeveloper)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("manager-role revocation did not stop another actor's volume import")
	}
	if !body.closed.Load() {
		t.Fatal("manager-role revocation did not close the blocked request body")
	}
	if content.committed.Load() {
		t.Fatal("volume import committed after manager-role revocation")
	}
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("revoked volume import status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("auth.authorization_revoked")) {
		t.Fatalf("revoked volume import response = %s", recorder.Body.String())
	}
}

func TestVolumeImportAuthorizationAllowsSucceededCompletionWindow(t *testing.T) {
	handlers, state := newVolumeDownloadAuthorizationTestHandlers(t)
	state.transferDirection.Store(model.VolumeTransferDirectionImport)
	state.transferState.Store(model.VolumeTransferStateSucceeded)
	user := model.User{ID: "usr_download", Role: authz.PlatformRoleUser}
	if !handlers.volumeImportAuthorizationAllowed(t.Context(), user, volumeImportAuthorizationReference{
		ProjectID: "prj_download", TransferID: "vtx_download",
	}) {
		t.Fatal("succeeded import must remain authorized during the handler completion window")
	}
}

func TestVolumeDownloadAuthorizationRequiresCurrentExportRole(t *testing.T) {
	handlers, state := newVolumeDownloadAuthorizationTestHandlers(t)
	user := model.User{ID: "usr_download", Role: authz.PlatformRoleUser}
	reference := volumeTransferDownloadAuthorizationReference{ProjectID: "prj_download", TransferID: "vtx_download"}

	if !handlers.volumeTransferDownloadAuthorizationAllowed(t.Context(), user, reference) {
		t.Fatal("project Owner should retain export access")
	}
	state.role.Store(authz.ProjectRoleAdmin)
	if !handlers.volumeTransferDownloadAuthorizationAllowed(t.Context(), user, reference) {
		t.Fatal("project Admin should retain export access")
	}
	state.role.Store(authz.ProjectRoleDeveloper)
	if handlers.volumeTransferDownloadAuthorizationAllowed(t.Context(), user, reference) {
		t.Fatal("project Developer must be denied even when they created the transfer")
	}
	state.role.Store(authz.ProjectRoleViewer)
	if handlers.volumeTransferDownloadAuthorizationAllowed(t.Context(), user, reference) {
		t.Fatal("project Viewer must be denied even when they created the transfer")
	}
	state.role.Store(authz.ProjectRoleOwner)
	state.memberMissing.Store(true)
	if handlers.volumeTransferDownloadAuthorizationAllowed(t.Context(), user, reference) {
		t.Fatal("removed project member must be denied")
	}
	user.Role = authz.PlatformRoleAdmin
	if !handlers.volumeTransferDownloadAuthorizationAllowed(t.Context(), user, reference) {
		t.Fatal("platform administrator should bypass project membership")
	}
}

type authorizationBlockingReadCloser struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	closed  atomic.Bool
}

func newAuthorizationBlockingReadCloser() *authorizationBlockingReadCloser {
	return &authorizationBlockingReadCloser{started: make(chan struct{}), release: make(chan struct{})}
}

func (reader *authorizationBlockingReadCloser) Read([]byte) (int, error) {
	reader.once.Do(func() { close(reader.started) })
	<-reader.release
	return 0, context.Canceled
}

func (reader *authorizationBlockingReadCloser) Close() error {
	if reader.closed.CompareAndSwap(false, true) {
		close(reader.release)
	}
	return nil
}

func TestVolumeDownloadRoleRevocationCancelsBlockedOutput(t *testing.T) {
	handlers, state := newVolumeDownloadAuthorizationTestHandlers(t)
	user := model.User{ID: "usr_download", Role: authz.PlatformRoleUser}
	binding := runtimeTerminalAuthorizationBinding{UserID: user.ID, SubjectID: "ses_download", Deadline: time.Now().Add(time.Minute)}
	reference := volumeTransferDownloadAuthorizationReference{ProjectID: "prj_download", TransferID: "vtx_download"}
	streamCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var checks atomic.Int64
	revoked, active := handlers.monitorContinuousAuthorizationWithInterval(
		streamCtx,
		binding,
		func(checkCtx context.Context, currentUser model.User) bool {
			checks.Add(1)
			return handlers.volumeTransferDownloadAuthorizationAllowed(checkCtx, currentUser, reference)
		},
		cancel,
		5*time.Millisecond,
	)
	if !active {
		t.Fatal("initial download authorization unexpectedly failed")
	}

	body := newAuthorizationBlockingReadCloser()
	var output bytes.Buffer
	copyDone := make(chan error, 1)
	go func() { copyDone <- copyVolumeDownloadBody(streamCtx, &output, body, nil) }()
	select {
	case <-body.started:
	case <-time.After(time.Second):
		t.Fatal("download reader did not block")
	}
	deadline := time.Now().Add(time.Second)
	for checks.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if checks.Load() == 0 {
		t.Fatal("authorization monitor did not perform an initial resource check")
	}

	state.role.Store(authz.ProjectRoleViewer)
	select {
	case <-revoked:
	case <-time.After(time.Second):
		t.Fatal("role revocation did not cancel the download")
	}
	select {
	case err := <-copyDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("blocked copy error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked download continued after authorization revocation")
	}
	if !body.closed.Load() {
		t.Fatal("authorization cancellation did not close the download reader")
	}
	if output.Len() != 0 {
		t.Fatalf("download emitted %d bytes after revocation", output.Len())
	}
	if reason := volumeDownloadStreamFailureReason(streamCtx, revoked, io.ErrClosedPipe); reason != "authorization_revoked" {
		t.Fatalf("audit reason = %q, want authorization_revoked", reason)
	}
}

func TestVolumeDownloadFailureReasonDoesNotExposeRawError(t *testing.T) {
	secretError := errors.New("provider token=super-secret")
	if reason := volumeDownloadStreamFailureReason(context.Background(), nil, secretError); reason != "stream_interrupted" {
		t.Fatalf("audit reason = %q", reason)
	}
}
