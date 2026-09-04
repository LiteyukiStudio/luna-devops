package api

import (
	"bytes"
	"context"
	"errors"
	"github.com/LiteyukiStudio/devops/internal/api/notificationapi"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/gin-gonic/gin"
)

type fakeInboxService struct {
	listInput         inbox.ListInput
	listResult        inbox.ListResult
	message           model.InboxMessage
	actionRequest     model.InboxActionRequest
	actionRequests    map[string]model.InboxActionRequest
	unreadCount       int64
	markReadUserID    string
	markReadMessageID string
	markAllReadUserID string
	archiveUserID     string
	archiveMessageID  string
	err               error
}

func (f *fakeInboxService) List(_ context.Context, input inbox.ListInput) (inbox.ListResult, error) {
	f.listInput = input
	return f.listResult, f.err
}

func (f *fakeInboxService) Get(_ context.Context, _, _ string) (model.InboxMessage, error) {
	return f.message, f.err
}

func (f *fakeInboxService) GetActionRequest(_ context.Context, _, _ string) (model.InboxActionRequest, error) {
	return f.actionRequest, f.err
}

func (f *fakeInboxService) GetActionRequests(_ context.Context, _ string, _ []string) (map[string]model.InboxActionRequest, error) {
	return f.actionRequests, f.err
}

func (f *fakeInboxService) UnreadCount(_ context.Context, _ string) (int64, error) {
	return f.unreadCount, f.err
}

func (f *fakeInboxService) MarkRead(_ context.Context, userID, messageID string) error {
	f.markReadUserID = userID
	f.markReadMessageID = messageID
	return f.err
}

func (f *fakeInboxService) MarkAllRead(_ context.Context, userID string) error {
	f.markAllReadUserID = userID
	return f.err
}

func (f *fakeInboxService) Archive(_ context.Context, userID, messageID string) error {
	f.archiveUserID = userID
	f.archiveMessageID = messageID
	return f.err
}

func TestListInboxMessagesNormalizesFiltersAndPagination(t *testing.T) {
	service := &fakeInboxService{listResult: inbox.ListResult{
		Items:      []model.InboxMessage{},
		Page:       2,
		PageSize:   100,
		SortBy:     "createdAt",
		SortOrder:  "asc",
		Total:      0,
		TotalPages: 0,
	}}
	handlers := &Handlers{inbox: service}
	handlers.domains = newDomainHandlers(handlers)
	recorder, ctx := newInboxHandlerContext(http.MethodGet, "/api/v1/inbox?page=2&pageSize=500&sortBy=unsafe&sortOrder=asc&filter=unread&category=billing", "usr_current", nil)

	handlers.domains.notification.ListInboxMessages(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if service.listInput.UserID != "usr_current" || service.listInput.Page != 2 || service.listInput.PageSize != 100 {
		t.Fatalf("list input identity/pagination = %#v", service.listInput)
	}
	if service.listInput.Filter != "unread" || service.listInput.Category != "billing" {
		t.Fatalf("list filters = %#v", service.listInput)
	}
	if service.listInput.SortBy != "createdAt" || service.listInput.SortOrder != "asc" {
		t.Fatalf("list sort = %#v", service.listInput)
	}
	if cacheControl := recorder.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q", cacheControl)
	}
}

func TestInboxMessageResponseExposesParsedParamsWithoutStorageFields(t *testing.T) {
	response := notificationapi.InboxMessageResponseFor(model.InboxMessage{
		ID:              "imsg_1",
		RecipientUserID: "usr_private",
		ParamsJSON:      `{"actorName":"Snowy","count":2}`,
		DedupKey:        stringPointer("private-deduplication-key"),
	})
	encoded := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(encoded)
	ctx.JSON(http.StatusOK, response)
	if !strings.Contains(encoded.Body.String(), `"params":{"actorName":"Snowy","count":2}`) {
		t.Fatalf("response = %s", encoded.Body.String())
	}
	for _, forbidden := range []string{"paramsJson", "recipientUserId", "dedup", "private-deduplication-key"} {
		if strings.Contains(encoded.Body.String(), forbidden) {
			t.Fatalf("response exposes %q: %s", forbidden, encoded.Body.String())
		}
	}

	invalid := notificationapi.InboxMessageResponseFor(model.InboxMessage{ParamsJSON: "invalid"})
	if invalid.Params == nil || len(invalid.Params) != 0 {
		t.Fatalf("invalid params = %#v, want empty object", invalid.Params)
	}
}

func TestListInboxMessagesRejectsUnknownFilterBeforeQuery(t *testing.T) {
	service := &fakeInboxService{}
	handlers := &Handlers{inbox: service}
	handlers.domains = newDomainHandlers(handlers)
	recorder, ctx := newInboxHandlerContext(http.MethodGet, "/api/v1/inbox?filter=everything", "usr_current", nil)

	handlers.domains.notification.ListInboxMessages(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if service.listInput.UserID != "" {
		t.Fatalf("service was called with %#v", service.listInput)
	}
}

func TestInboxMutationsAlwaysUseCurrentUser(t *testing.T) {
	service := &fakeInboxService{}
	handlers := &Handlers{inbox: service}
	handlers.domains = newDomainHandlers(handlers)

	readRecorder, readContext := newInboxHandlerContext(http.MethodPost, "/api/v1/inbox/imsg_other/read", "usr_current", nil)
	readContext.Params = gin.Params{{Key: "messageId", Value: "imsg_other"}}
	handlers.domains.notification.MarkInboxMessageRead(readContext)
	if readRecorder.Code != http.StatusNoContent || service.markReadUserID != "usr_current" || service.markReadMessageID != "imsg_other" {
		t.Fatalf("read status=%d user=%q message=%q", readRecorder.Code, service.markReadUserID, service.markReadMessageID)
	}

	archiveRecorder, archiveContext := newInboxHandlerContext(http.MethodPost, "/api/v1/inbox/imsg_other/archive", "usr_current", nil)
	archiveContext.Params = gin.Params{{Key: "messageId", Value: "imsg_other"}}
	handlers.domains.notification.ArchiveInboxMessage(archiveContext)
	if archiveRecorder.Code != http.StatusNoContent || service.archiveUserID != "usr_current" || service.archiveMessageID != "imsg_other" {
		t.Fatalf("archive status=%d user=%q message=%q", archiveRecorder.Code, service.archiveUserID, service.archiveMessageID)
	}
}

func TestDecideInboxActionRequestUsesInjectedBusinessHook(t *testing.T) {
	service := &fakeInboxService{actionRequest: model.InboxActionRequest{
		ID:         "iar_1",
		Type:       "project.billing_owner_transfer",
		Status:     "completed",
		RowVersion: 2,
	}}
	var gotUserID, gotRequestID, gotDecision string
	var gotVersion int64
	handlers := &Handlers{
		inbox: service,
		inboxDecision: func(_ context.Context, user model.User, requestID, decision string, expectedVersion int64) error {
			gotUserID = user.ID
			gotRequestID = requestID
			gotDecision = decision
			gotVersion = expectedVersion
			return nil
		},
	}
	handlers.domains = newDomainHandlers(handlers)
	body := strings.NewReader(`{"decision":"accept","expectedVersion":1}`)
	recorder, ctx := newInboxHandlerContext(http.MethodPost, "/api/v1/inbox/action-requests/iar_1/decision", "usr_current", body)
	ctx.Params = gin.Params{{Key: "requestId", Value: "iar_1"}}

	handlers.domains.notification.DecideInboxActionRequest(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if gotUserID != "usr_current" || gotRequestID != "iar_1" || gotDecision != "accept" || gotVersion != 1 {
		t.Fatalf("hook args = user=%q request=%q decision=%q version=%d", gotUserID, gotRequestID, gotDecision, gotVersion)
	}
	if !strings.Contains(recorder.Body.String(), `"allowedDecisions":[]`) {
		t.Fatalf("completed response = %s", recorder.Body.String())
	}
}

func TestDecideInboxActionRequestRejectsInvalidInput(t *testing.T) {
	called := false
	handlers := &Handlers{
		inbox: &fakeInboxService{},
		inboxDecision: func(context.Context, model.User, string, string, int64) error {
			called = true
			return nil
		},
	}
	handlers.domains = newDomainHandlers(handlers)
	recorder, ctx := newInboxHandlerContext(http.MethodPost, "/api/v1/inbox/action-requests/iar_1/decision", "usr_current", strings.NewReader(`{"decision":"approve","expectedVersion":0}`))
	ctx.Params = gin.Params{{Key: "requestId", Value: "iar_1"}}

	handlers.domains.notification.DecideInboxActionRequest(ctx)

	if recorder.Code != http.StatusBadRequest || called {
		t.Fatalf("status=%d called=%t body=%s", recorder.Code, called, recorder.Body.String())
	}
}

func TestInboxChangeBrokerIsolatesUsersAndUnsubscribes(t *testing.T) {
	broker := notificationapi.NewInboxChangeBroker()
	userOne, unsubscribeOne := broker.Subscribe("usr_one")
	userTwo, unsubscribeTwo := broker.Subscribe("usr_two")
	defer unsubscribeTwo()

	broker.Notify("usr_one", "imsg_1")
	select {
	case change := <-userOne:
		if change.MessageID != "imsg_1" {
			t.Fatalf("messageID = %q", change.MessageID)
		}
	case <-time.After(time.Second):
		t.Fatal("user one did not receive their invalidation")
	}
	select {
	case change := <-userTwo:
		t.Fatalf("user two received cross-user invalidation: %#v", change)
	default:
	}

	unsubscribeOne()
	broker.Notify("usr_one", "imsg_2")
	select {
	case change := <-userOne:
		t.Fatalf("unsubscribed channel received invalidation: %#v", change)
	default:
	}
}

func TestStreamInboxChangesStartsWithDatabaseUnreadCount(t *testing.T) {
	service := &fakeInboxService{unreadCount: 4}
	handlers := &Handlers{inbox: service}
	handlers.domains = newDomainHandlers(handlers)
	recorder, ctx := newInboxHandlerContext(http.MethodGet, "/api/v1/inbox/stream", "usr_current", nil)
	requestContext, cancel := context.WithCancel(ctx.Request.Context())
	cancel()
	ctx.Request = ctx.Request.WithContext(requestContext)

	handlers.domains.notification.StreamInboxChanges(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "text/event-stream" {
		t.Fatalf("Content-Type = %q", contentType)
	}
	if cacheControl := recorder.Header().Get("Cache-Control"); cacheControl != "no-cache, no-store" {
		t.Fatalf("Cache-Control = %q", cacheControl)
	}
	if !strings.Contains(recorder.Body.String(), "event: inbox.changed\ndata: {\"unreadCount\":4}") {
		t.Fatalf("initial stream event = %q", recorder.Body.String())
	}
}

func TestWriteInboxChangedEventUsesStableSSEEnvelope(t *testing.T) {
	var output bytes.Buffer
	if err := notificationapi.WriteInboxChangedEvent(&output, notificationapi.InboxChangedEvent{UnreadCount: 3, MessageID: "imsg_1"}); err != nil {
		t.Fatal(err)
	}
	want := "event: inbox.changed\ndata: {\"unreadCount\":3,\"messageId\":\"imsg_1\"}\n\n"
	if output.String() != want {
		t.Fatalf("event = %q, want %q", output.String(), want)
	}
}

func TestWriteInboxErrorMapsStableSentinels(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{err: inbox.ErrInvalidInput, status: http.StatusBadRequest, code: "inbox.request_invalid"},
		{err: inbox.ErrNotFound, status: http.StatusNotFound, code: "inbox.not_found"},
		{err: projectservice.ErrBillingOwnerTransferInvalid, status: http.StatusBadRequest, code: "inbox.billing_owner_transfer_invalid"},
		{err: projectservice.ErrBillingOwnerTransferForbidden, status: http.StatusForbidden, code: "inbox.billing_owner_transfer_forbidden"},
		{err: projectservice.ErrBillingOwnerTransferConflict, status: http.StatusConflict, code: "inbox.billing_owner_transfer_conflict"},
		{err: projectservice.ErrBillingOwnerTransferStale, status: http.StatusConflict, code: "inbox.billing_owner_transfer_stale"},
		{err: projectservice.ErrBillingOwnerTransferNotFound, status: http.StatusNotFound, code: "inbox.billing_owner_transfer_not_found"},
		{err: projectservice.ErrBillingOwnerTransferExpired, status: http.StatusGone, code: "inbox.billing_owner_transfer_expired"},
		{err: errors.New("database unavailable"), status: http.StatusInternalServerError, code: "inbox.operation_failed"},
	} {
		recorder, ctx := newInboxHandlerContext(http.MethodGet, "/api/v1/inbox", "usr_current", nil)
		notificationapi.WriteInboxError(ctx, test.err)
		if recorder.Code != test.status || jsonString(t, recorder.Body.Bytes(), "code") != test.code {
			t.Fatalf("error=%v status=%d body=%s", test.err, recorder.Code, recorder.Body.String())
		}
	}
}

func TestInboxRoutesAreRegistered(t *testing.T) {
	db := authIntegrationDB(t)
	router := NewRouter(db, mustTestConfig(t))
	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, expected := range []string{
		"GET /api/v1/inbox",
		"GET /api/v1/inbox/unread-count",
		"GET /api/v1/inbox/stream",
		"GET /api/v1/inbox/:messageId",
		"POST /api/v1/inbox/:messageId/read",
		"POST /api/v1/inbox/read-all",
		"POST /api/v1/inbox/:messageId/archive",
		"POST /api/v1/inbox/action-requests/:requestId/decision",
		"POST /api/v1/projects/:projectId/billing-owner-transfer-requests",
	} {
		if !routes[expected] {
			t.Fatalf("route %q is not registered", expected)
		}
	}
}

func newInboxHandlerContext(method, target, userID string, body *strings.Reader) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	if body == nil {
		ctx.Request = httptest.NewRequest(method, target, nil)
	} else {
		ctx.Request = httptest.NewRequest(method, target, body)
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	ctx.Set(currentUserContextKey, model.User{ID: userID, Language: "zh-CN"})
	return recorder, ctx
}

func stringPointer(value string) *string {
	return &value
}
