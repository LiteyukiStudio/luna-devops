package notificationapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

type memSecretStore struct {
	seq  int
	data map[string]string
}

func newMemSecretStore() *memSecretStore {
	return &memSecretStore{data: map[string]string{}}
}

func (m *memSecretStore) ResolveContext(_ context.Context, ref string) string {
	return m.data[strings.TrimSpace(ref)]
}

func (m *memSecretStore) StoreContext(_ context.Context, plaintext, _, _ string) string {
	m.seq++
	ref := fmt.Sprintf("sec_%d", m.seq)
	m.data[ref] = plaintext
	return ref
}

func (m *memSecretStore) StoreContextWithDB(ctx context.Context, _ *gorm.DB, plaintext, createdBy, resource string) (string, error) {
	return m.StoreContext(ctx, plaintext, createdBy, resource), nil
}

func (m *memSecretStore) DeleteRefContextWithDB(_ context.Context, _ *gorm.DB, ref, _ string) error {
	delete(m.data, strings.TrimSpace(ref))
	return nil
}

type noopDeliveryEnqueuer struct {
	enqueued []string
}

func (e *noopDeliveryEnqueuer) EnqueueNotificationDeliver(_ context.Context, payload tasks.NotificationDeliverPayload) (*asynq.TaskInfo, error) {
	e.enqueued = append(e.enqueued, payload.DeliveryID)
	return &asynq.TaskInfo{ID: payload.DeliveryID}, nil
}

type stubNotificationAIHost struct {
	db       *gorm.DB
	user     model.User
	secrets  *memSecretStore
	enqueuer *noopDeliveryEnqueuer
	inboxSvc InboxService
}

func (s *stubNotificationAIHost) DBFor(*gin.Context) *gorm.DB { return s.db }
func (s *stubNotificationAIHost) DBWithContext(ctx context.Context) *gorm.DB {
	if s.db == nil {
		return nil
	}
	return s.db.WithContext(ctx)
}
func (s *stubNotificationAIHost) CurrentUser(*gin.Context) (model.User, bool) {
	return s.user, s.user.ID != ""
}
func (s *stubNotificationAIHost) RequirePlatformAdmin(ctx *gin.Context) bool {
	if !authz.IsPlatformAdmin(s.user.Role) {
		writeErrorCode(ctx, http.StatusForbidden, "auth.forbidden", "admin required")
		return false
	}
	return true
}
func (s *stubNotificationAIHost) AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return s.AuthorizeProjectRef(ctx, ctx.Param("projectId"), action)
}
func (s *stubNotificationAIHost) AuthorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool) {
	return s.AuthorizeProjectRef(ctx, projectID, action)
}
func (s *stubNotificationAIHost) AuthorizeProjectRef(ctx *gin.Context, projectRef string, action authz.Action) (model.User, model.Project, bool) {
	proj, err := aitool.ResolveProject(s.db, projectRef)
	if err != nil {
		writeErrorCode(ctx, http.StatusNotFound, "project.not_found", err.Error())
		return model.User{}, model.Project{}, false
	}
	allowed, _ := s.ProjectRoleActionAllowed(ctx.Request.Context(), s.user, proj.ID, action)
	if !allowed {
		writeErrorCode(ctx, http.StatusForbidden, "auth.forbidden", "forbidden")
		return model.User{}, model.Project{}, false
	}
	return s.user, proj, true
}
func (s *stubNotificationAIHost) EnsureProjectCanMutate(*gin.Context, model.Project) bool {
	return true
}
func (s *stubNotificationAIHost) ProjectRoleActionAllowed(_ context.Context, user model.User, projectID string, action authz.Action) (bool, error) {
	if authz.IsPlatformAdmin(user.Role) {
		return true, nil
	}
	var m model.ProjectMember
	if err := s.db.First(&m, "project_id = ? AND user_id = ?", projectID, user.ID).Error; err != nil {
		return false, nil
	}
	allowedActions := authz.ProjectActionsAllowed(user.Role, m.Role)
	return slices.Contains(allowedActions, action), nil
}
func (s *stubNotificationAIHost) ProjectIDsForUser(_ context.Context, userID string) []string {
	var ids []string
	_ = s.db.Model(&model.ProjectMember{}).Where("user_id = ?", userID).Pluck("project_id", &ids).Error
	return ids
}
func (s *stubNotificationAIHost) ReplaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, _ []string) error {
	if err := tx.Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
		return err
	}
	for _, pid := range normalizeUniqueStrings(projectIDs) {
		if err := tx.Create(&model.ScopedResourceProjectBinding{
			ID:           id.New("srpb"),
			ResourceType: resourceType,
			ResourceID:   resourceID,
			ProjectID:    pid,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}
func (s *stubNotificationAIHost) ScopedResourceProjectIDs(resourceType, resourceID string, _ context.Context) []string {
	var ids []string
	_ = s.db.Model(&model.ScopedResourceProjectBinding{}).
		Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).
		Order("project_id asc").
		Pluck("project_id", &ids).Error
	return ids
}
func (s *stubNotificationAIHost) ScopedResourceProjectIDMap(resourceType string, resourceIDs []string, _ context.Context) map[string][]string {
	out := map[string][]string{}
	if len(resourceIDs) == 0 {
		return out
	}
	var rows []model.ScopedResourceProjectBinding
	_ = s.db.Where("resource_type = ? AND resource_id IN ?", resourceType, resourceIDs).Order("project_id asc").Find(&rows).Error
	for _, r := range rows {
		out[r.ResourceID] = append(out[r.ResourceID], r.ProjectID)
	}
	return out
}
func (s *stubNotificationAIHost) WriteProjectAuthorizationError(ctx *gin.Context, _ error) {
	writeErrorCode(ctx, http.StatusForbidden, "auth.forbidden", "forbidden")
}
func (s *stubNotificationAIHost) AuditWithContext(string, string, string, bool, string, context.Context) {
}
func (s *stubNotificationAIHost) Secrets() SecretStore { return s.secrets }
func (s *stubNotificationAIHost) DeliveryEnqueuer() notification.DeliveryEnqueuer {
	return s.enqueuer
}
func (s *stubNotificationAIHost) NormalizeRegistrationEmail(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if !strings.Contains(value, "@") {
		return "", fmt.Errorf("invalid email")
	}
	return value, nil
}
func (s *stubNotificationAIHost) AllowPersonalNotificationTest(*gin.Context, string) bool {
	return true
}
func (s *stubNotificationAIHost) InboxService() InboxService { return s.inboxSvc }
func (s *stubNotificationAIHost) InboxDecisionAvailable() bool {
	return true
}
func (s *stubNotificationAIHost) DecideInboxAction(ctx context.Context, user model.User, requestID, decision string, expectedVersion int64) error {
	_, err := (projectservice.BillingOwnerTransferService{DB: s.db}).Decide(ctx, user.ID, requestID, decision, expectedVersion)
	return err
}

func newNotifToolCtx(db *gorm.DB, user model.User, convProjectID string, locked any) *aitool.ToolContext {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools", nil)
	var lockedRaw json.RawMessage
	if locked != nil {
		lockedRaw, _ = json.Marshal(locked)
	}
	return aitool.NewToolContext(c, db, user, "ses_test", "conv_test", convProjectID, "run_test", "tc_test", "idem_test", "model", lockedRaw)
}

func TestNotificationAIToolsCatalogRegistrationAndSchemas(t *testing.T) {
	reg := aitool.NewRegistry()
	New(&stubNotificationAIHost{secrets: newMemSecretStore()}).RegisterAITools(reg)

	if reg.Len() != 23 {
		t.Fatalf("expected 23 registered tools in notificationapi, got %d", reg.Len())
	}
	if err := reg.ValidateCrossReferences(); err != nil {
		t.Fatalf("ValidateCrossReferences failed: %v", err)
	}

	type expectedSpec struct {
		domain    string
		tier      string
		effect    string
		adminOnly bool
		sensitive []string
	}
	expected := map[string]expectedSpec{
		"ops_list_inbox":                              {aitool.DomainOps, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"ops_get_my_notifications":                    {aitool.DomainOps, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"ops_list_project_notifications":              {aitool.DomainOps, aitool.ApprovalTierL0, aitool.EffectRead, false, nil},
		"ops_mark_inbox_read":                         {aitool.DomainOps, aitool.ApprovalTierL1, aitool.EffectWrite, false, nil},
		"ops_archive_inbox_message":                   {aitool.DomainOps, aitool.ApprovalTierL2, aitool.EffectWrite, false, nil},
		"ops_decide_inbox_action":                     {aitool.DomainOps, aitool.ApprovalTierL2, aitool.EffectWrite, false, nil},
		"ops_save_my_notification_channel":            {aitool.DomainOps, aitool.ApprovalTierL2, aitool.EffectWrite, false, []string{"upsertSecrets.*"}},
		"ops_delete_my_notification_channel":          {aitool.DomainOps, aitool.ApprovalTierL2, aitool.EffectWrite, false, nil},
		"ops_configure_my_notification_preferences":   {aitool.DomainOps, aitool.ApprovalTierL1, aitool.EffectWrite, false, nil},
		"ops_test_or_retry_my_notification":           {aitool.DomainOps, aitool.ApprovalTierL2, aitool.EffectExecute, false, nil},
		"ops_save_project_notification_channel":       {aitool.DomainOps, aitool.ApprovalTierL2, aitool.EffectWrite, false, []string{"upsertSecrets.*"}},
		"ops_unbind_project_notification_channel":     {aitool.DomainOps, aitool.ApprovalTierL2, aitool.EffectWrite, false, nil},
		"ops_delete_project_notification_channel":     {aitool.DomainOps, aitool.ApprovalTierL2, aitool.EffectWrite, false, nil},
		"ops_save_project_notification_rule":          {aitool.DomainOps, aitool.ApprovalTierL2, aitool.EffectWrite, false, nil},
		"ops_delete_project_notification_rule":        {aitool.DomainOps, aitool.ApprovalTierL2, aitool.EffectWrite, false, nil},
		"ops_test_or_retry_project_notification":      {aitool.DomainOps, aitool.ApprovalTierL2, aitool.EffectExecute, false, nil},
		"admin_list_platform_notifications":           {aitool.DomainAdmin, aitool.ApprovalTierL0, aitool.EffectRead, true, nil},
		"admin_configure_platform_mail":               {aitool.DomainAdmin, aitool.ApprovalTierL2, aitool.EffectExecute, true, []string{"password"}},
		"admin_save_global_notification_channel":      {aitool.DomainAdmin, aitool.ApprovalTierL2, aitool.EffectWrite, true, []string{"upsertSecrets.*"}},
		"admin_save_notification_template":            {aitool.DomainAdmin, aitool.ApprovalTierL2, aitool.EffectWrite, true, nil},
		"admin_save_global_notification_rule":         {aitool.DomainAdmin, aitool.ApprovalTierL2, aitool.EffectWrite, true, nil},
		"admin_test_or_retry_global_notification":     {aitool.DomainAdmin, aitool.ApprovalTierL2, aitool.EffectExecute, true, nil},
		"admin_delete_platform_notification_resource": {aitool.DomainAdmin, aitool.ApprovalTierL2, aitool.EffectWrite, true, nil},
	}

	for name, want := range expected {
		tool, ok := reg.Get(name)
		if !ok {
			t.Fatalf("missing tool %q", name)
		}
		cat := tool.Catalog
		if cat.Domain != want.domain || cat.ApprovalTier != want.tier || cat.Effect != want.effect || cat.AdminOnly != want.adminOnly {
			t.Fatalf("tool %s metadata mismatch: got domain=%s tier=%s effect=%s adminOnly=%v",
				name, cat.Domain, cat.ApprovalTier, cat.Effect, cat.AdminOnly)
		}
		if !slices.Equal(cat.SensitivePaths, want.sensitive) {
			t.Fatalf("tool %s sensitivePaths = %#v, want %#v", name, cat.SensitivePaths, want.sensitive)
		}
		checkRole := authz.PlatformRoleUser
		if want.adminOnly {
			checkRole = authz.PlatformRoleAdmin
		}
		for _, sc := range cat.RequiredScopes {
			if authz.NormalizeOAuthScope(sc) != sc || !authz.UserCanAuthorizeOAuthScope(checkRole, sc) {
				t.Fatalf("tool %s has invalid or unauthorized scope %q for role %s", name, sc, checkRole)
			}
		}
	}

	// Verify polymorphic whenArgumentEquals verification on admin_delete_platform_notification_resource
	delTool, _ := reg.Get("admin_delete_platform_notification_resource")
	if len(delTool.Catalog.Verification) != 3 {
		t.Fatalf("expected 3 polymorphic verification rules on admin_delete_platform_notification_resource, got %d", len(delTool.Catalog.Verification))
	}
	for _, wantType := range []string{"channel", "template", "rule"} {
		matched := false
		for _, v := range delTool.Catalog.Verification {
			if v.OperationID == "admin_list_platform_notifications" && v.Expect == "absent" && v.WhenArgumentEquals["resourceType"] == wantType {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("missing whenArgumentEquals[resourceType=%s] verification rule", wantType)
		}
	}
}

func TestNotificationAIToolsExecutionAndSecurityContracts(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "notif_ai_tools_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.User{},
				&model.Project{},
				&model.ProjectMember{},
				&model.Application{},
				&model.DeploymentTarget{},
				&model.InboxMessage{},
				&model.InboxActionRequest{},
				&model.PlatformEvent{},
				&model.PlatformMailSettings{},
				&model.NotificationChannel{},
				&model.NotificationTemplate{},
				&model.NotificationRule{},
				&model.NotificationDelivery{},
				&model.UserNotificationPreference{},
				&model.ScopedResourceProjectBinding{},
				&model.AuditLog{},
			)
		},
	})

	admin := model.User{ID: "usr_admin", Email: "admin@example.com", Name: "Admin", Role: authz.PlatformRoleAdmin}
	owner := model.User{ID: "usr_owner", Email: "owner@example.com", Name: "Owner", Role: authz.PlatformRoleUser}
	member := model.User{ID: "usr_member", Email: "member@example.com", Name: "Member", Role: authz.PlatformRoleUser}
	_ = db.Create(&[]model.User{admin, owner, member}).Error

	prj1 := model.Project{ID: "prj_1", Identifier: "proj-one", Name: "Proj One", BillingOwnerUserID: owner.ID, DeleteStatus: "active"}
	prj2 := model.Project{ID: "prj_2", Identifier: "proj-two", Name: "Proj Two", BillingOwnerUserID: owner.ID, DeleteStatus: "active"}
	_ = db.Create(&[]model.Project{prj1, prj2}).Error
	_ = db.Create(&[]model.ProjectMember{
		{ID: "pm_1", ProjectID: prj1.ID, UserID: owner.ID, Role: authz.ProjectRoleOwner},
		{ID: "pm_2", ProjectID: prj1.ID, UserID: member.ID, Role: authz.ProjectRoleOwner},
		{ID: "pm_3", ProjectID: prj2.ID, UserID: owner.ID, Role: authz.ProjectRoleOwner},
	}).Error

	secrets := newMemSecretStore()
	enqueuer := &noopDeliveryEnqueuer{}
	inboxSvc := inbox.NewService(db)
	host := &stubNotificationAIHost{
		db:       db,
		user:     owner,
		secrets:  secrets,
		enqueuer: enqueuer,
		inboxSvc: inboxSvc,
	}
	h := New(host)
	reg := aitool.NewRegistry()
	h.RegisterAITools(reg)

	mustExec := func(user model.User, convProj, toolName string, payload any) map[string]any {
		t.Helper()
		host.user = user
		raw, _ := json.Marshal(payload)
		tool, _ := reg.Get(toolName)
		var locked any
		if tool.HasPreview {
			pr, err := tool.Preview(newNotifToolCtx(db, user, convProj, nil), raw)
			if err != nil {
				t.Fatalf("%s preview failed: %v", toolName, err)
			}
			locked = pr.LockedPayload
		}
		res, err := tool.Execute(newNotifToolCtx(db, user, convProj, locked), raw)
		if err != nil {
			t.Fatalf("%s execute failed: %v", toolName, err)
		}
		encoded, _ := json.Marshal(res)
		var out map[string]any
		_ = json.Unmarshal(encoded, &out)
		return out
	}

	// 1. Inbox list, mark read, archive (and 404 on archived single-lookup!), and decide action
	transferReq, err := (projectservice.BillingOwnerTransferService{DB: db}).Request(context.Background(), owner.ID, prj1.ID, member.ID)
	if err != nil {
		t.Fatalf("create billing transfer request: %v", err)
	}

	memberInbox := mustExec(member, "", "ops_list_inbox", map[string]any{"filter": "action"})
	if memberInbox["unreadCount"] != float64(1) {
		t.Fatalf("expected unreadCount=1, got %#v", memberInbox)
	}
	msgID := memberInbox["items"].([]any)[0].(map[string]any)["id"].(string)

	decideOut := mustExec(member, "", "ops_decide_inbox_action", map[string]any{
		"requestId":       transferReq.ID,
		"decision":        "accept",
		"expectedVersion": transferReq.RowVersion,
	})
	if decideOut["status"] != "completed" || decideOut["messageId"] != msgID {
		t.Fatalf("unexpected decide result: %#v", decideOut)
	}

	markReadOut := mustExec(member, "", "ops_mark_inbox_read", map[string]any{"messageId": msgID})
	if markReadOut["markedAll"] != false {
		t.Fatalf("unexpected markRead output: %#v", markReadOut)
	}

	archOut := mustExec(member, "", "ops_archive_inbox_message", map[string]any{"messageId": msgID})
	if archOut["archived"] != true {
		t.Fatalf("unexpected archive output: %#v", archOut)
	}
	listInboxTool, _ := reg.Get("ops_list_inbox")
	if _, err := listInboxTool.Execute(newNotifToolCtx(db, member, "", nil), json.RawMessage(fmt.Sprintf(`{"messageId":%q}`, msgID))); err == nil {
		t.Fatalf("expected archived inbox message lookup by messageId to return 404")
	}

	// 2. Personal notification preferences, channels, secret redaction, and retry status gate
	prefOut := mustExec(owner, "", "ops_configure_my_notification_preferences", map[string]any{
		"emailEnabled":     false,
		"removeEventTypes": []string{"hook.failed"},
	})
	if prefOut["emailEnabled"] != false {
		t.Fatalf("expected emailEnabled=false, got %#v", prefOut)
	}

	myCh := mustExec(owner, "", "ops_save_my_notification_channel", map[string]any{
		"name":          "My Feishu",
		"presetId":      "feishu-bot",
		"upsertSecrets": map[string]string{"WebhookToken": "tok_secret_123", "SignSecret": "sig_secret_456"},
	})
	myChID := myCh["id"].(string)
	myChRaw, _ := json.Marshal(myCh)
	if strings.Contains(string(myChRaw), "tok_secret_123") || strings.Contains(string(myChRaw), "sig_secret_456") {
		t.Fatalf("secret leaked in response: %s", string(myChRaw))
	}

	// Clear optional SignSecret while preserving WebhookToken
	myChPatched := mustExec(owner, "", "ops_save_my_notification_channel", map[string]any{
		"channel":         "My Feishu",
		"clearSecretKeys": []string{"SignSecret"},
	})
	secSet := myChPatched["secretSet"].(map[string]any)
	if secSet["WebhookToken"] != true || secSet["SignSecret"] == true {
		t.Fatalf("expected WebhookToken preserved and SignSecret cleared, got %#v", secSet)
	}

	// Retry gate: succeeded delivery must return 409 notification.delivery_not_retryable
	_ = db.Create(&model.NotificationDelivery{ID: "ndl_ok", RecipientUserID: owner.ID, EventID: "ev_1", EventType: "build.failed", ChannelID: myChID, AdapterKind: "webhook", Status: "succeeded"}).Error
	_ = db.Create(&model.NotificationDelivery{ID: "ndl_fail", RecipientUserID: owner.ID, EventID: "ev_2", EventType: "build.failed", ChannelID: myChID, AdapterKind: "webhook", Status: "failed"}).Error

	retryMyTool, _ := reg.Get("ops_test_or_retry_my_notification")
	if _, err := retryMyTool.Preview(newNotifToolCtx(db, owner, "", nil), json.RawMessage(`{"retryDeliveryId":"ndl_ok"}`)); err == nil {
		t.Fatalf("expected succeeded delivery retry to fail with 409")
	}
	retryRes := mustExec(owner, "", "ops_test_or_retry_my_notification", map[string]any{"retryDeliveryId": "ndl_fail"})
	if retryRes["mode"] != "retry" || !slices.Contains(enqueuer.enqueued, "ndl_fail") {
		t.Fatalf("unexpected retry result: %#v", retryRes)
	}
	mustExec(owner, "", "ops_delete_my_notification_channel", map[string]any{"channel": myChID})

	// 3. Project notification channels: R11 plain-secret guard, union-bind, single-project sandbox, unbind, delete, rules
	saveProjChTool, _ := reg.Get("ops_save_project_notification_channel")
	if _, err := saveProjChTool.Preview(newNotifToolCtx(db, owner, "", nil), json.RawMessage(`{
		"project":"proj-one",
		"name":"Bad Webhook",
		"adapterKind":"webhook",
		"upsertConfig":{"url":"https://hooks.slack.com/services/T/B/X","headers":{"Authorization":"Bearer literal_secret"}}
	}`)); err == nil {
		t.Fatalf("expected literal Authorization header without {{.Secrets.*}} to be rejected")
	}

	projCh := mustExec(owner, "", "ops_save_project_notification_channel", map[string]any{
		"project":       "proj-one",
		"name":          "Shared Slack",
		"presetId":      "slack-webhook",
		"upsertSecrets": map[string]string{"WebhookPath": "T00/B00/SECRET"},
	})
	projChID := projCh["id"].(string)

	// Union-bind into proj-two inside single-project conversation (pure bind allowed!)
	boundOut := mustExec(owner, prj2.ID, "ops_save_project_notification_channel", map[string]any{
		"project": "proj-two",
		"channel": projChID,
	})
	if len(boundOut["projectIds"].([]any)) != 2 {
		t.Fatalf("expected union-bound projectIds length 2, got %#v", boundOut["projectIds"])
	}

	// Single-project session must block config mutation on multi-project shared channel!
	if _, err := saveProjChTool.Preview(newNotifToolCtx(db, owner, prj2.ID, nil), json.RawMessage(fmt.Sprintf(`{
		"project":"proj-two","channel":%q,"name":"Hijack Name"
	}`, projChID))); err == nil {
		t.Fatalf("expected single-project conversation to block mutating multi-project shared channel")
	}

	// Delete must block while shared across 2 projects (409 in global session)
	delProjChTool, _ := reg.Get("ops_delete_project_notification_channel")
	if _, err := delProjChTool.Preview(newNotifToolCtx(db, owner, "", nil), json.RawMessage(fmt.Sprintf(`{
		"project":"proj-one","channel":%q
	}`, projChID))); err == nil {
		t.Fatalf("expected deleting multi-project channel to return 409")
	}

	// Unbind from proj-two -> now exclusive to proj-one
	unboundOut := mustExec(owner, "", "ops_unbind_project_notification_channel", map[string]any{
		"project": "proj-two",
		"channel": projChID,
	})
	if unboundOut["unbound"] != true || unboundOut["projectId"] != prj2.ID {
		t.Fatalf("unexpected unbind response: %#v", unboundOut)
	}

	// Create & delete project notification rule
	projRule := mustExec(owner, "", "ops_save_project_notification_rule", map[string]any{
		"project":       "proj-one",
		"name":          "Prod Failures",
		"addEventTypes": []string{"release.failed"},
		"addChannels":   []string{"Shared Slack"},
	})
	projRuleID := projRule["id"].(string)
	if projRule["projectId"] != prj1.ID {
		t.Fatalf("expected projectId echoed on rule: %#v", projRule)
	}

	// Channel referenced by rule -> delete blocked
	if _, err := delProjChTool.Preview(newNotifToolCtx(db, owner, "", nil), json.RawMessage(fmt.Sprintf(`{
		"project":"proj-one","channel":%q
	}`, projChID))); err == nil {
		t.Fatalf("expected deleting rule-referenced channel to fail with 409")
	}
	mustExec(owner, "", "ops_delete_project_notification_rule", map[string]any{"project": "proj-one", "rule": projRuleID})
	delChOut := mustExec(owner, "", "ops_delete_project_notification_channel", map[string]any{"project": "proj-one", "channel": projChID})
	if delChOut["deleted"] != true || delChOut["projectId"] != prj1.ID {
		t.Fatalf("unexpected delete channel output: %#v", delChOut)
	}

	// 4. Admin platform mail & global notification resources
	mailOut := mustExec(admin, "", "admin_configure_platform_mail", map[string]any{
		"host":                         "smtp.corp.example",
		"port":                         587,
		"security":                     "starttls",
		"username":                     "mailer",
		"password":                     "smtp_secret_pw",
		"fromAddress":                  "noreply@corp.example",
		"personalEmailCooldownSeconds": 120,
	})
	if mailOut["passwordSet"] != true || mailOut["personalEmailCooldownSeconds"] != float64(120) {
		t.Fatalf("unexpected mail settings response: %#v", mailOut)
	}
	mailCleared := mustExec(admin, "", "admin_configure_platform_mail", map[string]any{
		"clearUsername": true,
		"clearPassword": true,
	})
	if mailCleared["passwordSet"] != false || mailCleared["username"] != "" {
		t.Fatalf("expected clearUsername+clearPassword to reset auth, got %#v", mailCleared)
	}

	globalCh := mustExec(admin, "", "admin_save_global_notification_channel", map[string]any{
		"name":          "Global WeCom",
		"presetId":      "wecom-bot",
		"upsertSecrets": map[string]string{"WebhookKey": "wecom_key_999"},
	})
	globalChID := globalCh["id"].(string)

	// Verify preset channel creation did NOT insert a redundant NotificationTemplate row
	var tplCount int64
	_ = db.Model(&model.NotificationTemplate{}).Count(&tplCount).Error
	if tplCount != 0 {
		t.Fatalf("expected 0 extra NotificationTemplate rows on preset channel creation, got %d", tplCount)
	}

	tpl := mustExec(admin, "", "admin_save_notification_template", map[string]any{
		"name":             "Custom Webhook Alert",
		"eventType":        "release.failed",
		"adapterKind":      "webhook",
		"jsonBodyTemplate": `{"text":"{{.Event.Message}}"}`,
	})
	tplID := tpl["id"].(string)

	globalRule := mustExec(admin, "", "admin_save_global_notification_rule", map[string]any{
		"name":          "Platform Critical Alerts",
		"addEventTypes": []string{"release.failed"},
		"addChannels":   []string{"Global WeCom"},
		"template":      "Custom Webhook Alert",
		"filter": map[string]any{
			"scope":       "projects",
			"addProjects": []string{"proj-one"},
		},
	})
	globalRuleID := globalRule["id"].(string)

	// Verify admin_list_platform_notifications(section=deliveries) includes global-rule project-event deliveries and excludes project-rule deliveries
	_ = db.Create(&model.NotificationRule{ID: "nrl_proj_only", ProjectID: prj1.ID, Name: "Tenant Rule"}).Error
	_ = db.Create(&model.NotificationDelivery{ID: "ndl_global_on_proj", ProjectID: prj1.ID, EventID: "ev_g1", EventType: "release.failed", ChannelID: globalChID, RuleID: globalRuleID, AdapterKind: "webhook", Status: "failed"}).Error
	_ = db.Create(&model.NotificationDelivery{ID: "ndl_tenant_only", ProjectID: prj1.ID, EventID: "ev_t1", EventType: "release.failed", ChannelID: globalChID, RuleID: "nrl_proj_only", AdapterKind: "webhook", Status: "failed"}).Error

	adminDelivs := mustExec(admin, "", "admin_list_platform_notifications", map[string]any{"section": "deliveries"})
	if adminDelivs["total"] != float64(1) || adminDelivs["items"].([]any)[0].(map[string]any)["id"] != "ndl_global_on_proj" {
		t.Fatalf("expected only global-rule delivery in admin_list_platform_notifications, got %#v", adminDelivs)
	}

	// Delete global rule -> template -> channel
	delRuleRes := mustExec(admin, "", "admin_delete_platform_notification_resource", map[string]any{
		"resourceType": "rule",
		"resource":     globalRuleID,
	})
	if delRuleRes["section"] != "rules" || delRuleRes["deleted"] != true {
		t.Fatalf("unexpected delete rule response: %#v", delRuleRes)
	}
	delTplRes := mustExec(admin, "", "admin_delete_platform_notification_resource", map[string]any{
		"resourceType": "template",
		"resource":     tplID,
	})
	if delTplRes["section"] != "templates" || delTplRes["deleted"] != true {
		t.Fatalf("unexpected delete template response: %#v", delTplRes)
	}
	delGlobalChRes := mustExec(admin, "", "admin_delete_platform_notification_resource", map[string]any{
		"resourceType": "channel",
		"resource":     globalChID,
	})
	if delGlobalChRes["section"] != "channels" || delGlobalChRes["deleted"] != true {
		t.Fatalf("unexpected delete global channel response: %#v", delGlobalChRes)
	}
}

type fakeInboxService struct {
	messages map[string]model.InboxMessage
	actions  map[string]model.InboxActionRequest
}

func (f *fakeInboxService) List(_ context.Context, input inbox.ListInput) (inbox.ListResult, error) {
	var items []model.InboxMessage
	for _, m := range f.messages {
		if m.RecipientUserID == input.UserID && m.ArchivedAt == nil {
			items = append(items, m)
		}
	}
	return inbox.ListResult{Items: items, Page: 1, PageSize: 20, SortBy: "createdAt", SortOrder: "desc", Total: int64(len(items)), TotalPages: 1}, nil
}

func (f *fakeInboxService) Get(_ context.Context, userID, messageID string) (model.InboxMessage, error) {
	m, ok := f.messages[messageID]
	if !ok || m.RecipientUserID != userID {
		return model.InboxMessage{}, inbox.ErrNotFound
	}
	return m, nil
}

func (f *fakeInboxService) GetActionRequest(_ context.Context, userID, requestID string) (model.InboxActionRequest, error) {
	a, ok := f.actions[requestID]
	if !ok || a.RecipientUserID != userID {
		return model.InboxActionRequest{}, inbox.ErrNotFound
	}
	return a, nil
}

func (f *fakeInboxService) GetActionRequests(_ context.Context, userID string, ids []string) (map[string]model.InboxActionRequest, error) {
	out := map[string]model.InboxActionRequest{}
	for _, idVal := range ids {
		if a, ok := f.actions[idVal]; ok && a.RecipientUserID == userID {
			out[idVal] = a
		}
	}
	return out, nil
}

func (f *fakeInboxService) UnreadCount(_ context.Context, userID string) (int64, error) {
	var c int64
	for _, m := range f.messages {
		if m.RecipientUserID == userID && m.ArchivedAt == nil && m.ReadAt == nil {
			c++
		}
	}
	return c, nil
}

func (f *fakeInboxService) MarkRead(_ context.Context, userID, messageID string) error {
	m, ok := f.messages[messageID]
	if !ok || m.RecipientUserID != userID {
		return inbox.ErrNotFound
	}
	now := time.Now()
	m.ReadAt = &now
	f.messages[messageID] = m
	return nil
}

func (f *fakeInboxService) MarkAllRead(_ context.Context, userID string) error {
	now := time.Now()
	for k, m := range f.messages {
		if m.RecipientUserID == userID && m.ReadAt == nil {
			m.ReadAt = &now
			f.messages[k] = m
		}
	}
	return nil
}

func (f *fakeInboxService) Archive(_ context.Context, userID, messageID string) error {
	m, ok := f.messages[messageID]
	if !ok || m.RecipientUserID != userID {
		return inbox.ErrNotFound
	}
	now := time.Now()
	m.ArchivedAt = &now
	f.messages[messageID] = m
	return nil
}

func TestNotificationAIToolsPureSecurityGuardsAndInboxLifecycle(t *testing.T) {
	// 1. R11 anti-plaintext-secret guard on upsertConfig
	if err := validateUpsertConfigNoPlainSecrets(map[string]any{"password": "plain"}); err == nil {
		t.Fatalf("expected password in upsertConfig to be rejected")
	}
	if err := validateUpsertConfigNoPlainSecrets(map[string]any{"url": "https://user:pass@example.com/hook"}); err == nil {
		t.Fatalf("expected URL userinfo credentials in upsertConfig to be rejected")
	}
	if err := validateUpsertConfigNoPlainSecrets(map[string]any{
		"url":     "https://api.example.com/hook",
		"headers": map[string]any{"Authorization": "Bearer raw-secret-token"},
	}); err == nil {
		t.Fatalf("expected literal Authorization header without {{.Secrets.*}} to be rejected")
	}
	if err := validateUpsertConfigNoPlainSecrets(map[string]any{
		"url":     "https://api.example.com/hook",
		"headers": map[string]any{"Authorization": "Bearer {{.Secrets.BearerToken}}"},
	}); err != nil {
		t.Fatalf("expected templated Authorization header with {{.Secrets.*}} to succeed, got %v", err)
	}

	// 2. Delivery retry status gate
	now := time.Now()
	recentStart := now.Add(-10 * time.Second)
	staleStart := now.Add(-2 * time.Hour)
	for _, nonRetryable := range []model.NotificationDelivery{
		{ID: "d1", Status: "succeeded"},
		{ID: "d2", Status: "pending"},
		{ID: "d3", Status: "retry_pending"},
		{ID: "d4", Status: "sending", StartedAt: &recentStart},
	} {
		if err := ensureDeliveryRetryable(nonRetryable, now); err == nil {
			t.Fatalf("expected delivery status=%q to be rejected with 409", nonRetryable.Status)
		}
	}
	for _, retryable := range []model.NotificationDelivery{
		{ID: "d5", Status: "failed"},
		{ID: "d6", Status: "enqueue_failed"},
		{ID: "d7", Status: "sending", StartedAt: &staleStart},
	} {
		if err := ensureDeliveryRetryable(retryable, now); err != nil {
			t.Fatalf("expected delivery status=%q to be retryable, got %v", retryable.Status, err)
		}
	}

	// 3. Platform mail patch semantics (omitted fields preserved + clearUsername/clearPassword)
	existingMail := model.PlatformMailSettings{
		ID:                           "default",
		Host:                         "smtp.example.com",
		Port:                         587,
		Security:                     "starttls",
		Username:                     "mailer",
		PasswordRef:                  "sec_pw",
		FromAddress:                  "noreply@example.com",
		FromName:                     "Luna Ops",
		PersonalEmailCooldownSeconds: 90,
	}
	newCooldown := 180
	patchedMail, _, _, mutated, _, _, err := preparePlatformMailPatch(existingMail, adminConfigurePlatformMailInput{
		PersonalEmailCooldownSeconds: &newCooldown,
	})
	if err != nil || !mutated || patchedMail.PasswordRef != "sec_pw" || patchedMail.Username != "mailer" || patchedMail.PersonalEmailCooldownSeconds != 180 {
		t.Fatalf("expected omitted mail fields & passwordRef preserved, got %#v (%v)", patchedMail, err)
	}
	clearTrue := true
	clearedMail, _, clearPass, _, sensitiveKeys, _, err := preparePlatformMailPatch(existingMail, adminConfigurePlatformMailInput{
		ClearUsername: &clearTrue,
		ClearPassword: &clearTrue,
	})
	if err != nil || !clearPass || clearedMail.Username != "" || clearedMail.PasswordRef != "" || !slices.Equal(sensitiveKeys, []string{"password"}) {
		t.Fatalf("expected clearUsername+clearPassword to succeed cleanly, got %#v (%v)", clearedMail, err)
	}

	// 4. Inbox tools end-to-end with in-memory InboxService (including archived 404 check & expectedVersion lock)
	actor := model.User{ID: "usr_1", Email: "u1@example.com", Name: "U1", Role: authz.PlatformRoleUser}
	fakeInbox := &fakeInboxService{
		messages: map[string]model.InboxMessage{
			"msg_1": {ID: "msg_1", RecipientUserID: actor.ID, Type: "project.billing_owner_transfer_requested", Category: "action", ActionRequestID: "areq_1", ParamsJSON: `{}`},
		},
		actions: map[string]model.InboxActionRequest{
			"areq_1": {ID: "areq_1", RecipientUserID: actor.ID, RequesterUserID: "usr_2", ProjectID: "prj_1", Type: "project.billing_owner_transfer", Status: "pending", RowVersion: 3},
		},
	}
	host := &stubNotificationAIHost{user: actor, secrets: newMemSecretStore(), inboxSvc: fakeInbox}
	reg := aitool.NewRegistry()
	New(host).RegisterAITools(reg)

	decideTool, _ := reg.Get("ops_decide_inbox_action")
	// Stale version must fail in Preview!
	if _, err := decideTool.Preview(newNotifToolCtx(nil, actor, "", nil), json.RawMessage(`{"requestId":"areq_1","decision":"accept","expectedVersion":1}`)); err == nil {
		t.Fatalf("expected stale expectedVersion=1 to fail in Preview")
	}
	pr, err := decideTool.Preview(newNotifToolCtx(nil, actor, "", nil), json.RawMessage(`{"requestId":"areq_1","decision":"accept","expectedVersion":3}`))
	if err != nil || pr.LockedPayload == nil {
		t.Fatalf("expected valid expectedVersion=3 Preview to lock payload, got %v", err)
	}

	archiveTool, _ := reg.Get("ops_archive_inbox_message")
	archPreview, err := archiveTool.Preview(newNotifToolCtx(nil, actor, "", nil), json.RawMessage(`{"messageId":"msg_1"}`))
	if err != nil {
		t.Fatalf("archive preview failed: %v", err)
	}
	if _, err := archiveTool.Execute(newNotifToolCtx(nil, actor, "", archPreview.LockedPayload), json.RawMessage(`{"messageId":"msg_1"}`)); err != nil {
		t.Fatalf("archive execute failed: %v", err)
	}

	listTool, _ := reg.Get("ops_list_inbox")
	if _, err := listTool.Execute(newNotifToolCtx(nil, actor, "", nil), json.RawMessage(`{"messageId":"msg_1"}`)); err == nil {
		t.Fatalf("expected ops_list_inbox(messageId) to return 404 after archiving")
	}
}
