package volumeapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/api/projectapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type volumeAITestHost struct {
	db             *gorm.DB
	currentUserVal model.User
	audits         []string
}

func (m *volumeAITestHost) DBWithContext(ctx context.Context) *gorm.DB {
	return m.db.WithContext(ctx)
}
func (m *volumeAITestHost) CurrentUser(_ *gin.Context) (model.User, bool) {
	return m.currentUserVal, m.currentUserVal.ID != ""
}
func (m *volumeAITestHost) AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return m.AuthorizeProjectRef(ctx, ctx.Param("projectId"), action)
}
func (m *volumeAITestHost) AuthorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool) {
	return m.AuthorizeProjectRef(ctx, projectID, action)
}
func (m *volumeAITestHost) AuthorizeProjectRef(ctx *gin.Context, projectRef string, _ authz.Action) (model.User, model.Project, bool) {
	proj, err := aitool.ResolveProject(m.db, projectRef)
	if err != nil {
		if te, ok := aitool.AsToolError(err); ok {
			writeErrorCode(ctx, te.Status, te.Code, te.Message)
		} else {
			writeErrorCode(ctx, http.StatusNotFound, "project.not_found", err.Error())
		}
		return model.User{}, model.Project{}, false
	}
	return m.currentUserVal, proj, true
}
func (m *volumeAITestHost) ApplyScopedResourceVisibilityForProject(query *gorm.DB, _ string, _ model.User, _ string, _ context.Context) *gorm.DB {
	return query
}
func (m *volumeAITestHost) ProjectMemberActionAllowed(_ *gin.Context, _, _ string, _ authz.Action) (bool, bool) {
	return true, true
}
func (m *volumeAITestHost) ProjectRoleActionAllowed(_ context.Context, _ model.User, _ string, _ authz.Action) (bool, error) {
	return true, nil
}
func (m *volumeAITestHost) ProjectAuthorizer(_ context.Context) authz.ProjectAuthorizer { return nil }
func (m *volumeAITestHost) RuntimeClusterForProjectUse(ctx *gin.Context, _ model.User, _, clusterID string) (model.RuntimeCluster, bool) {
	var c model.RuntimeCluster
	if err := m.db.First(&c, "id = ?", clusterID).Error; err != nil {
		writeErrorCode(ctx, http.StatusNotFound, "runtime_cluster.not_found", "cluster not found")
		return model.RuntimeCluster{}, false
	}
	return c, true
}
func (m *volumeAITestHost) EnsureBillingAllowsDeployChange(_ *gin.Context, _ string) bool {
	return true
}
func (m *volumeAITestHost) EnsureBillingAllowsManagedVolumeChange(_ *gin.Context, _ string) bool {
	return true
}
func (m *volumeAITestHost) AuditWithContext(_, action, resource string, _ bool, _ string, _ context.Context) {
	m.audits = append(m.audits, action+":"+resource)
}
func (m *volumeAITestHost) CurrentAccessTokenFromContext(_ *gin.Context) (model.AccessToken, bool) {
	return model.AccessToken{}, false
}
func (m *volumeAITestHost) AccessTokenAllows(scopeText, required string) bool {
	return authz.AccessTokenAllows(scopeText, required)
}
func (m *volumeAITestHost) RequestUsesBearerToken(_ *gin.Context) bool { return false }
func (m *volumeAITestHost) RequireContinuousAuthorizationBinding(_ *gin.Context, _ model.User) (projectapi.ContinuousAuthorizationBinding, bool) {
	return projectapi.ContinuousAuthorizationBinding{}, false
}
func (m *volumeAITestHost) CurrentInteractiveAuthorizationBinding(_ *gin.Context, _ model.User) (projectapi.ContinuousAuthorizationBinding, bool) {
	return projectapi.ContinuousAuthorizationBinding{}, false
}
func (m *volumeAITestHost) MonitorContinuousAuthorization(_ context.Context, _ projectapi.ContinuousAuthorizationBinding, _ func(context.Context, model.User) bool, _ func()) (<-chan struct{}, bool) {
	return nil, false
}
func (m *volumeAITestHost) ProjectContinuousAuthorizationAllowed(_ context.Context, _ model.User, _ string, _ authz.Action) bool {
	return true
}
func (m *volumeAITestHost) ResourceCanMutateDuringDelete(status string) bool {
	return status == "" || status == "active" || status == "delete_failed"
}

type stubVolumeClusterService struct {
	classes    []projectVolumeStorageClass
	classesErr error
	claimSpec  volume.ExistingClaimInspection
	claimErr   error
	dispatched []volume.VolumeOperation
}

func (s *stubVolumeClusterService) InspectExistingClaim(_ context.Context, _ volume.ExistingClaimInspectionInput) (volume.ExistingClaimInspection, error) {
	if s.claimErr != nil {
		return volume.ExistingClaimInspection{}, s.claimErr
	}
	return s.claimSpec, nil
}

func (s *stubVolumeClusterService) ListStorageClasses(_ context.Context, _ string) ([]projectVolumeStorageClass, error) {
	if s.classesErr != nil {
		return nil, s.classesErr
	}
	return append([]projectVolumeStorageClass(nil), s.classes...), nil
}

func (s *stubVolumeClusterService) ObserveProjectVolumes(_ context.Context, items []model.ProjectVolume) map[string]projectVolumeObservationResponse {
	out := make(map[string]projectVolumeObservationResponse, len(items))
	now := time.Now().UTC()
	for _, it := range items {
		out[it.ID] = projectVolumeObservationResponse{
			Status:       model.ProjectVolumeAvailabilityAvailable,
			Exists:       true,
			Phase:        "Bound",
			Capacity:     it.CapacityRequest,
			StorageClass: it.StorageClassName,
			AccessModes:  []string{it.AccessMode},
			VolumeMode:   it.VolumeMode,
			ObservedAt:   now,
		}
	}
	return out
}

func (s *stubVolumeClusterService) DispatchVolumeOperation(_ context.Context, op volume.VolumeOperation) error {
	s.dispatched = append(s.dispatched, op)
	return nil
}

type stubVolumeContentService struct {
	volumes       *volume.Service
	lastIdemKey   string
	lastExportReq volumeExportCreateInput
}

func (s *stubVolumeContentService) CreateImport(context.Context, model.User, model.Project, volumeImportCreateInput, string) (model.ProjectVolume, model.VolumeTransfer, error) {
	return model.ProjectVolume{}, model.VolumeTransfer{}, nil
}
func (s *stubVolumeContentService) StreamImport(context.Context, string, string, model.User, io.Reader, int64) (model.VolumeTransfer, error) {
	return model.VolumeTransfer{}, nil
}
func (s *stubVolumeContentService) CreateExport(ctx context.Context, user model.User, project model.Project, volumeID string, input volumeExportCreateInput, idemKey string) (model.VolumeTransfer, error) {
	s.lastIdemKey = idemKey
	s.lastExportReq = input
	return s.volumes.CreateVolumeTransfer(ctx, volume.CreateVolumeTransferInput{
		ProjectID:       project.ID,
		ProjectVolumeID: volumeID,
		Direction:       model.VolumeTransferDirectionExport,
		Format:          input.Format,
		ConsistencyMode: model.VolumeTransferConsistencyUnmounted,
		ActorID:         user.ID,
		ExpiresAt:       time.Now().UTC().Add(30 * time.Minute),
		IdempotencyKey:  idemKey,
	})
}
func (s *stubVolumeContentService) RetryTransfer(context.Context, model.User, model.Project, model.VolumeTransfer, string) (model.VolumeTransfer, error) {
	return model.VolumeTransfer{}, nil
}
func (s *stubVolumeContentService) AuthorizeDownload(context.Context, model.User, model.Project, model.VolumeTransfer, volumeDownloadBinding) (volumeDownloadAuthorizationResponse, error) {
	return volumeDownloadAuthorizationResponse{}, nil
}
func (s *stubVolumeContentService) OpenDownload(context.Context, model.User, model.Project, model.VolumeTransfer, string, volumeDownloadBinding) (volumeDownload, error) {
	return volumeDownload{}, nil
}
func (s *stubVolumeContentService) OpenManifest(context.Context, model.User, model.Project, model.VolumeTransfer, string, volumeDownloadBinding) (volumeDownload, error) {
	return volumeDownload{}, nil
}

func setupVolumeAITestEnv(t *testing.T) (*Handler, *stubVolumeClusterService, *stubVolumeContentService, *gorm.DB, model.User, model.Project, model.RuntimeCluster) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "volume_ai_tools_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.User{},
				&model.Project{},
				&model.RuntimeCluster{},
				&model.ScopedResourceProjectBinding{},
				&model.Application{},
				&model.DeploymentTarget{},
				&model.ProjectVolume{},
				&model.DeploymentVolumeMount{},
				&model.VolumeTransfer{},
				&model.AuditLog{},
			)
		},
	})
	user := model.User{ID: "usr_vol", Role: authz.PlatformRoleUser}
	proj := model.Project{ID: "prj_vol", Identifier: "vol-proj", Name: "Volume Project", KubernetesNamespace: "luna-vol", DeleteStatus: "active"}
	cluster := model.RuntimeCluster{ID: "clu_vol", Name: "primary-cluster", Scope: "global", DeleteStatus: "active"}
	if err := db.Create(&proj).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}

	clusterSvc := &stubVolumeClusterService{
		classes: []projectVolumeStorageClass{
			{Name: "fast-ssd", Provisioner: "csi.example.com", IsDefault: true, AllowVolumeExpansion: true, VolumeBindingMode: "WaitForFirstConsumer"},
		},
		claimSpec: volume.ExistingClaimInspection{
			CapacityRequest:  "5Gi",
			CapacityBytes:    5 * 1024 * 1024 * 1024,
			StorageClassName: "fast-ssd",
			AccessMode:       model.ProjectVolumeAccessReadWriteOnce,
			VolumeMode:       model.ProjectVolumeModeFilesystem,
		},
	}
	volSvc := volume.NewServiceWithDependencies(volume.NewGormRepository(db), clusterSvc, clusterSvc)
	contentSvc := &stubVolumeContentService{volumes: volSvc}
	host := &volumeAITestHost{db: db, currentUserVal: user}

	h := New(host, Dependencies{
		Volumes:          volSvc,
		Clusters:         clusterSvc,
		Content:          contentSvc,
		TransferMaxBytes: 10 * 1024 * 1024 * 1024,
		TransferEnabled:  true,
	})
	return h, clusterSvc, contentSvc, db, user, proj, cluster
}

func newVolumeTestToolContext(db *gorm.DB, user model.User, idemKey string, lockedPayload any) *aitool.ToolContext {
	rec := httptest.NewRecorder()
	parent, _ := gin.CreateTestContext(rec)
	parent.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools", nil)
	var rawLock json.RawMessage
	if lockedPayload != nil {
		rawLock, _ = json.Marshal(lockedPayload)
	}
	return aitool.NewToolContext(parent, db, user, "ses_vol", "conv_vol", "", "run_vol", "call_vol", idemKey, "model", rawLock)
}

func TestVolumeAIToolsRegistrationCatalogAndSchemas(t *testing.T) {
	h := &Handler{}
	reg := aitool.NewRegistry()
	h.RegisterAITools(reg)

	if reg.Len() != 8 {
		t.Fatalf("expected 8 registered tools in volumeapi, got %d", reg.Len())
	}

	type expectedSpec struct {
		tier           string
		effect         string
		retryOnUnknown bool
	}
	want := map[string]expectedSpec{
		"runtime_list_volumes":           {tier: aitool.ApprovalTierL0, effect: aitool.EffectRead},
		"runtime_get_volume":             {tier: aitool.ApprovalTierL0, effect: aitool.EffectRead},
		"runtime_create_volume":          {tier: aitool.ApprovalTierL2, effect: aitool.EffectAsync, retryOnUnknown: true},
		"runtime_update_volume":          {tier: aitool.ApprovalTierL2, effect: aitool.EffectAsync},
		"runtime_retry_volume_operation": {tier: aitool.ApprovalTierL2, effect: aitool.EffectAsync},
		"runtime_delete_volume":          {tier: aitool.ApprovalTierL2, effect: aitool.EffectAsync},
		"runtime_create_volume_export":   {tier: aitool.ApprovalTierL2, effect: aitool.EffectAsync, retryOnUnknown: true},
		"runtime_cancel_volume_transfer": {tier: aitool.ApprovalTierL2, effect: aitool.EffectWrite},
	}

	for name, spec := range want {
		tool, ok := reg.Get(name)
		if !ok {
			t.Fatalf("missing registered tool %q", name)
		}
		cat := tool.Catalog
		if cat.Domain != aitool.DomainRuntime || cat.ApprovalTier != spec.tier || cat.Effect != spec.effect || cat.RetryOnUnknown != spec.retryOnUnknown {
			t.Fatalf("%s metadata mismatch: got domain=%q tier=%q effect=%q retry=%v, want %+v",
				name, cat.Domain, cat.ApprovalTier, cat.Effect, cat.RetryOnUnknown, spec)
		}
		if len(cat.SensitivePaths) != 0 {
			t.Fatalf("%s unexpected sensitivePaths: %v", name, cat.SensitivePaths)
		}
		for _, sc := range cat.RequiredScopes {
			if !authz.UserCanAuthorizeOAuthScope(authz.PlatformRoleUser, sc) || !authz.UserCanAuthorizeOAuthScope(authz.PlatformRoleAdmin, sc) {
				t.Fatalf("%s declares invalid OAuth scope %q", name, sc)
			}
		}
		if props, ok := cat.InputSchema["properties"].(map[string]any); ok {
			if ps, hasPS := props["pageSize"].(map[string]any); hasPS {
				if ps["maximum"] != int64(50) || ps["default"] != int64(20) {
					t.Fatalf("%s pageSize schema = %#v, want default=20 maximum=50", name, ps)
				}
			}
		}
	}

	if err := reg.ValidateCrossReferences(); err != nil {
		t.Fatalf("ValidateCrossReferences failed: %v", err)
	}
}

func TestVolumeAIToolsEndToEndLifecycleLocksAndIdempotency(t *testing.T) {
	h, clusterSvc, contentSvc, db, user, proj, _ := setupVolumeAITestEnv(t)
	reg := aitool.NewRegistry()
	h.RegisterAITools(reg)

	listTool, _ := reg.Get("runtime_list_volumes")
	getTool, _ := reg.Get("runtime_get_volume")
	createTool, _ := reg.Get("runtime_create_volume")
	updateTool, _ := reg.Get("runtime_update_volume")
	retryTool, _ := reg.Get("runtime_retry_volume_operation")
	deleteTool, _ := reg.Get("runtime_delete_volume")
	exportTool, _ := reg.Get("runtime_create_volume_export")
	cancelTool, _ := reg.Get("runtime_cancel_volume_transfer")

	// 1) Create blank volume omitting storageClassName: auto-selects single default class "fast-ssd" and consumes Idempotency-Key.
	createArgs := json.RawMessage(`{
		"project": "vol-proj",
		"displayName": "pg-data",
		"cluster": "primary-cluster",
		"sourceType": "blank",
		"capacity": "10Gi"
	}`)
	tc := newVolumeTestToolContext(db, user, "aiidem_vol_create_fixed_key", nil)
	createPrev, err := createTool.Preview(tc, createArgs)
	if err != nil {
		t.Fatalf("preview create volume: %v", err)
	}
	tcExecCreate := newVolumeTestToolContext(db, user, "aiidem_vol_create_fixed_key", createPrev.LockedPayload)
	createdRaw, err := createTool.Execute(tcExecCreate, createArgs)
	if err != nil {
		t.Fatalf("execute create volume: %v", err)
	}
	created := createdRaw.(map[string]any)
	volID := created["id"].(string)
	if created["projectId"] != proj.ID || created["storageClassName"] != "fast-ssd" {
		t.Fatalf("unexpected created volume: %#v", created)
	}

	// Idempotent replay with same Idempotency-Key returns the exact same volume ID without duplicating PVC.
	replayedRaw, err := createTool.Execute(tcExecCreate, createArgs)
	if err != nil || replayedRaw.(map[string]any)["id"] != volID {
		t.Fatalf("expected idempotent replay to return %s, got %#v (err=%v)", volID, replayedRaw, err)
	}

	// When multiple default StorageClasses exist and storageClassName is omitted -> 400 volume.storage_class_required with candidates.
	clusterSvc.classes = []projectVolumeStorageClass{
		{Name: "ssd-a", IsDefault: true},
		{Name: "ssd-b", IsDefault: true},
	}
	if _, err := createTool.Preview(tc, json.RawMessage(`{
		"project": "vol-proj",
		"displayName": "ambig-sc",
		"cluster": "primary-cluster",
		"sourceType": "blank",
		"capacity": "5Gi"
	}`)); err == nil || !strings.Contains(err.Error(), "volume.storage_class_required") {
		t.Fatalf("expected volume.storage_class_required, got %v", err)
	}
	clusterSvc.classes = []projectVolumeStorageClass{{Name: "fast-ssd", IsDefault: true, AllowVolumeExpansion: true}}

	// Mark volume ready for update/export/delete tests.
	if err := db.Model(&model.ProjectVolume{}).Where("id = ?", volID).Updates(map[string]any{
		"lifecycle_state":   model.ProjectVolumeLifecycleReady,
		"pending_operation": "",
	}).Error; err != nil {
		t.Fatal(err)
	}

	// 2) List volumes with cluster includes storageClasses & sets Cache-Control: no-store.
	listTC := newVolumeTestToolContext(db, user, "", nil)
	listRaw, err := listTool.Execute(listTC, json.RawMessage(`{"project":"vol-proj","cluster":"primary-cluster"}`))
	if err != nil {
		t.Fatalf("list volumes: %v", err)
	}
	if listTC.ResponseHeaders.Get("Cache-Control") != "no-store" {
		t.Fatalf("expected Cache-Control: no-store on runtime_list_volumes")
	}
	listMap := listRaw.(map[string]any)
	if listMap["projectId"] != proj.ID || listMap["storageClassesStatus"] != "ready" || !slices.Equal([]string{"fast-ssd"}, []string{listMap["storageClasses"].([]projectVolumeStorageClass)[0].Name}) {
		t.Fatalf("unexpected list response: %#v", listMap)
	}

	// 3) Update volume locks expectedRevision in PreviewResult.LockedPayload and expands to 20Gi.
	updateArgs := json.RawMessage(`{"project":"vol-proj","volume":"pg-data","displayName":"pg-data-prod","capacity":"20Gi"}`)
	upPrev, err := updateTool.Preview(tc, updateArgs)
	if err != nil {
		t.Fatalf("preview update volume: %v", err)
	}
	lockData, ok := upPrev.LockedPayload.(aiVolumeRevisionLock)
	if !ok || lockData.VolumeID != volID || lockData.ExpectedRevision < 1 {
		t.Fatalf("expected locked revision payload, got %#v", upPrev.LockedPayload)
	}
	// Missing lock must fail with 428 ai.approval_required.
	if _, err := updateTool.Execute(newVolumeTestToolContext(db, user, "", nil), updateArgs); err == nil || !strings.Contains(err.Error(), "ai.approval_required") {
		t.Fatalf("expected ai.approval_required without lock, got %v", err)
	}
	updatedRaw, err := updateTool.Execute(newVolumeTestToolContext(db, user, "", upPrev.LockedPayload), updateArgs)
	if err != nil {
		t.Fatalf("execute update volume: %v", err)
	}
	updated := updatedRaw.(map[string]any)
	if updated["displayName"] != "pg-data-prod" || updated["capacity"] != "20Gi" {
		t.Fatalf("unexpected updated volume: %#v", updated)
	}

	// 4) Retry failed expand operation.
	if err := db.Model(&model.ProjectVolume{}).Where("id = ?", volID).Updates(map[string]any{
		"lifecycle_state":   model.ProjectVolumeLifecycleError,
		"pending_operation": volume.OperationExpand,
		"last_error_code":   volume.CodeClusterUnavailable,
	}).Error; err != nil {
		t.Fatal(err)
	}
	retryArgs := json.RawMessage(`{"project":"vol-proj","volume":"pg-data-prod"}`)
	retryPrev, err := retryTool.Preview(tc, retryArgs)
	if err != nil {
		t.Fatalf("preview retry volume: %v", err)
	}
	retriedRaw, err := retryTool.Execute(newVolumeTestToolContext(db, user, "", retryPrev.LockedPayload), retryArgs)
	if err != nil || retriedRaw.(map[string]any)["lifecycleState"] != model.ProjectVolumeLifecycleProvisioning {
		t.Fatalf("retry volume failed: %#v, err=%v", retriedRaw, err)
	}
	_ = db.Model(&model.ProjectVolume{}).Where("id = ?", volID).Updates(map[string]any{
		"lifecycle_state":   model.ProjectVolumeLifecycleReady,
		"pending_operation": "",
	}).Error

	// 5) Create volume export auto-derives tar_gz, consumes Idempotency-Key, and get_volume resolves by transferId alone!
	exportArgs := json.RawMessage(`{"project":"vol-proj","volume":"pg-data-prod"}`)
	if _, err := exportTool.Preview(tc, exportArgs); err != nil {
		t.Fatalf("preview export: %v", err)
	}
	exportOutRaw, err := exportTool.Execute(newVolumeTestToolContext(db, user, "aiidem_export_key_999", nil), exportArgs)
	if err != nil {
		t.Fatalf("execute export: %v", err)
	}
	exportOut := exportOutRaw.(map[string]any)
	txID := exportOut["transferId"].(string)
	if contentSvc.lastIdemKey != "aiidem_export_key_999" || contentSvc.lastExportReq.Format != "tar_gz" || exportOut["projectId"] != proj.ID {
		t.Fatalf("unexpected export result: %#v, req=%+v, idem=%q", exportOut, contentSvc.lastExportReq, contentSvc.lastIdemKey)
	}

	getByTxRaw, err := getTool.Execute(listTC, json.RawMessage(`{"project":"vol-proj","transferId":"`+txID+`"}`))
	if err != nil {
		t.Fatalf("get volume by transferId: %v", err)
	}
	getByTx := getByTxRaw.(map[string]any)
	if getByTx["id"] != volID || getByTx["projectId"] != proj.ID {
		t.Fatalf("unexpected get_volume(transferId) response: %#v", getByTx)
	}

	// 6) Delete volume is blocked while export transfer is active, then succeeds once cancelled!
	delArgs := json.RawMessage(`{"project":"vol-proj","volume":"pg-data-prod"}`)
	if _, err := deleteTool.Preview(tc, delArgs); err == nil || !strings.Contains(err.Error(), volume.CodeTransferStateConflict) {
		t.Fatalf("expected delete preview to block while transfer is active, got %v", err)
	}

	cancelArgs := json.RawMessage(`{"project":"vol-proj","transferId":"` + txID + `"}`)
	if _, err := cancelTool.Preview(tc, cancelArgs); err != nil {
		t.Fatalf("preview cancel transfer: %v", err)
	}
	cancelOutRaw, err := cancelTool.Execute(tc, cancelArgs)
	if err != nil || cancelOutRaw.(map[string]any)["state"] != model.VolumeTransferStateCancelled {
		t.Fatalf("cancel transfer failed: %#v, err=%v", cancelOutRaw, err)
	}
	// Simulate asynchronous Worker finishing transfer cleanup after cancellation.
	if err := db.Model(&model.VolumeTransfer{}).Where("id = ?", txID).Update("execution_cleanup_completed_at", time.Now().UTC()).Error; err != nil {
		t.Fatal(err)
	}

	// Also verify active binding blocks deletion with 409 volume.in_use.
	mp := "/data"
	mount := model.DeploymentVolumeMount{
		ID:                 id.New("dvm"),
		ProjectID:          proj.ID,
		ApplicationID:      "app_1",
		DeploymentTargetID: "dplt_1",
		SourceType:         model.DeploymentVolumeSourceProjectVolume,
		ProjectVolumeID:    &volID,
		LogicalName:        "data",
		MountPath:          &mp,
		ActivationState:    model.DeploymentVolumeActivationActive,
	}
	if err := db.Create(&mount).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := deleteTool.Preview(tc, delArgs); err == nil || !strings.Contains(err.Error(), volume.CodeInUse) {
		t.Fatalf("expected 409 volume.in_use when active mount exists, got %v", err)
	}
	_ = db.Unscoped().Delete(&mount).Error

	delPrev, err := deleteTool.Preview(tc, delArgs)
	if err != nil {
		t.Fatalf("preview delete volume: %v", err)
	}
	delLock, ok := delPrev.LockedPayload.(aiVolumeDeleteLock)
	if !ok || delLock.DataAction != "delete" || len(delPrev.Preview.Warnings) == 0 {
		t.Fatalf("unexpected delete lock or preview: lock=%+v preview=%+v", delPrev.LockedPayload, delPrev.Preview)
	}
	delOutRaw, err := deleteTool.Execute(newVolumeTestToolContext(db, user, "", delPrev.LockedPayload), delArgs)
	if err != nil {
		t.Fatalf("execute delete volume: %v", err)
	}
	delOut := delOutRaw.(map[string]any)
	if delOut["id"] != volID || delOut["dataAction"] != "delete" || delOut["lifecycleState"] != model.ProjectVolumeLifecycleDeleting {
		t.Fatalf("unexpected delete volume response: %#v", delOut)
	}
}

func TestVolumeAIToolsPureHelpersAndFormatDerivation(t *testing.T) {
	// 1) Idempotency-Key prefers header and falls back deterministically to (runId, toolCallId).
	rec := httptest.NewRecorder()
	parent, _ := gin.CreateTestContext(rec)
	parent.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools", nil)

	tcWithHeader := aitool.NewToolContext(parent, nil, model.User{ID: "usr_1"}, "s1", "c1", "", "run_a", "call_a", "aiidem_explicit_header_key", "model", nil)
	key, err := effectiveVolumeIdempotencyKey(tcWithHeader, "volcreate")
	if err != nil || key != "aiidem_explicit_header_key" {
		t.Fatalf("expected explicit header key, got %q (err=%v)", key, err)
	}

	tcFallbackA := aitool.NewToolContext(parent, nil, model.User{ID: "usr_1"}, "s1", "c1", "", "run_a", "call_a", "", "model", nil)
	tcFallbackB := aitool.NewToolContext(parent, nil, model.User{ID: "usr_1"}, "s1", "c1", "", "run_a", "call_a", "", "model", nil)
	keyA, errA := effectiveVolumeIdempotencyKey(tcFallbackA, "volcreate")
	keyB, errB := effectiveVolumeIdempotencyKey(tcFallbackB, "volcreate")
	if errA != nil || errB != nil || keyA != keyB || !strings.HasPrefix(keyA, "aiidem_volcreate_") {
		t.Fatalf("expected deterministic fallback key, got %q vs %q (%v, %v)", keyA, keyB, errA, errB)
	}

	// 2) Export format auto-derivation & Block live consistency guard.
	fsVol := model.ProjectVolume{VolumeMode: model.ProjectVolumeModeFilesystem}
	fmtFS, consFS, err := resolveExportFormatAndConsistency(fsVol, nil, nil)
	if err != nil || fmtFS != model.VolumeTransferFormatTarGZ || consFS != "auto" {
		t.Fatalf("unexpected Filesystem export defaults: fmt=%q cons=%q err=%v", fmtFS, consFS, err)
	}

	blkVol := model.ProjectVolume{VolumeMode: model.ProjectVolumeModeBlock}
	fmtBlk, consBlk, err := resolveExportFormatAndConsistency(blkVol, nil, nil)
	if err != nil || fmtBlk != model.VolumeTransferFormatRawZST || consBlk != "auto" {
		t.Fatalf("unexpected Block export defaults: fmt=%q cons=%q err=%v", fmtBlk, consBlk, err)
	}

	live := "live"
	if _, _, err := resolveExportFormatAndConsistency(blkVol, nil, &live); err == nil {
		t.Fatal("expected Block volume with consistency=live to be rejected")
	}
	tarGZ := model.VolumeTransferFormatTarGZ
	if _, _, err := resolveExportFormatAndConsistency(blkVol, &tarGZ, nil); err == nil || !strings.Contains(err.Error(), volume.CodeTransferFormatMismatch) {
		t.Fatalf("expected format mismatch for Block+tar_gz, got %v", err)
	}
}
