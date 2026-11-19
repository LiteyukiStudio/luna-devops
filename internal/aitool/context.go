package aitool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ToolContext carries verified actor identity, trace context, database handle,
// conversation project sandbox, idempotency key, and L2 preview lock payload
// into domain AI tool Preview and Execute handlers.
type ToolContext struct {
	ParentGin             *gin.Context
	Gin                   *gin.Context
	Recorder              *httptest.ResponseRecorder
	DB                    *gorm.DB
	User                  model.User
	SessionID             string
	ConversationID        string
	ConversationProjectID string
	RunID                 string
	ToolCallID            string
	IdempotencyKey        string
	InputMode             string
	LockedPayload         json.RawMessage
	ResponseHeaders       http.Header
}

func NewToolContext(
	parent *gin.Context,
	db *gorm.DB,
	user model.User,
	sessionID, conversationID, conversationProjectID, runID, toolCallID, idempotencyKey, inputMode string,
	lockedPayload json.RawMessage,
) *ToolContext {
	reqCtx := context.Background()
	if parent != nil && parent.Request != nil {
		reqCtx = parent.Request.Context()
	}
	subGin, rec := newChildGinContext(parent, reqCtx, http.MethodPost, "/internal/v1/ai/tools", nil, nil, nil, idempotencyKey)
	var boundDB *gorm.DB
	if db != nil {
		boundDB = db.WithContext(reqCtx)
	}
	return &ToolContext{
		ParentGin:             parent,
		Gin:                   subGin,
		Recorder:              rec,
		DB:                    boundDB,
		User:                  user,
		SessionID:             sessionID,
		ConversationID:        conversationID,
		ConversationProjectID: conversationProjectID,
		RunID:                 runID,
		ToolCallID:            toolCallID,
		IdempotencyKey:        strings.TrimSpace(idempotencyKey),
		InputMode:             inputMode,
		LockedPayload:         lockedPayload,
		ResponseHeaders:       make(http.Header),
	}
}

func (tc *ToolContext) Context() context.Context {
	if tc != nil && tc.Gin != nil && tc.Gin.Request != nil {
		return tc.Gin.Request.Context()
	}
	if tc != nil && tc.ParentGin != nil && tc.ParentGin.Request != nil {
		return tc.ParentGin.Request.Context()
	}
	return context.Background()
}

func (tc *ToolContext) SetNoStore() {
	if tc != nil && tc.ResponseHeaders != nil {
		tc.ResponseHeaders.Set("Cache-Control", "no-store")
	}
}

func (tc *ToolContext) UnmarshalLockedPayload(dst any) error {
	if tc == nil || len(bytes.TrimSpace(tc.LockedPayload)) == 0 {
		return ErrPreviewLockMissing
	}
	if err := json.Unmarshal(tc.LockedPayload, dst); err != nil {
		return ErrPreviewLockInvalid
	}
	return nil
}

// AbortedError converts an error written onto tc.Gin by a reused domain helper
// into a structured *ToolError.
func (tc *ToolContext) AbortedError(fallbackCode, fallbackMessage string) error {
	if tc == nil || tc.Recorder == nil {
		return NewError(http.StatusBadRequest, fallbackCode, fallbackMessage)
	}
	return errorFromRecorder(tc.Recorder, fallbackCode, fallbackMessage)
}

// InvokeHandler executes an existing Gin domain handler in-process on an isolated
// child *gin.Context while preserving W3C trace context, authenticated User, and
// Idempotency-Key. Non-2xx responses are returned as *ToolError.
func (tc *ToolContext) InvokeHandler(
	method string,
	targetPath string,
	params gin.Params,
	query url.Values,
	body any,
	handler gin.HandlerFunc,
) (json.RawMessage, error) {
	var bodyBytes []byte
	if body != nil {
		switch typed := body.(type) {
		case []byte:
			bodyBytes = typed
		case json.RawMessage:
			bodyBytes = typed
		default:
			encoded, err := json.Marshal(body)
			if err != nil {
				return nil, BadRequest("request.invalid_json", err.Error())
			}
			bodyBytes = encoded
		}
	}
	childGin, rec := newChildGinContext(tc.Gin, tc.Context(), method, targetPath, params, query, bodyBytes, tc.IdempotencyKey)
	handler(childGin)

	if cc := rec.Header().Get("Cache-Control"); cc != "" && tc.ResponseHeaders != nil {
		tc.ResponseHeaders.Set("Cache-Control", cc)
	}

	status := rec.Code
	if status == 0 {
		status = childGin.Writer.Status()
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, errorFromRecorder(rec, "ai.tool_failed", "domain handler rejected request")
	}
	trimmed := bytes.TrimSpace(rec.Body.Bytes())
	if len(trimmed) == 0 {
		return json.RawMessage(`{}`), nil
	}
	return json.RawMessage(append([]byte(nil), trimmed...)), nil
}

// InvokeJSON executes an existing Gin domain handler and unmarshals its 2xx JSON response into dst.
func (tc *ToolContext) InvokeJSON(
	method string,
	targetPath string,
	params gin.Params,
	query url.Values,
	body any,
	handler gin.HandlerFunc,
	dst any,
) error {
	raw, err := tc.InvokeHandler(method, targetPath, params, query, body, handler)
	if err != nil {
		return err
	}
	if dst == nil {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

func newChildGinContext(
	parent *gin.Context,
	reqCtx context.Context,
	method string,
	targetPath string,
	params gin.Params,
	query url.Values,
	bodyBytes []byte,
	idempotencyKey string,
) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	subGin, _ := gin.CreateTestContext(rec)
	fullURL := targetPath
	if fullURL == "" {
		fullURL = "/internal/v1/ai/tools"
	}
	if len(query) > 0 {
		sep := "?"
		if strings.Contains(fullURL, "?") {
			sep = "&"
		}
		fullURL += sep + query.Encode()
	}
	var bodyReader io.Reader = http.NoBody
	if bodyBytes != nil {
		bodyReader = bytes.NewReader(bodyBytes)
	}
	req, _ := http.NewRequestWithContext(reqCtx, method, fullURL, bodyReader)
	if parent != nil && parent.Request != nil {
		for k, vals := range parent.Request.Header {
			if strings.EqualFold(k, "Authorization") ||
				strings.EqualFold(k, "X-Luna-AI-Run-ID") ||
				strings.EqualFold(k, "X-Luna-AI-Tool-Call-ID") ||
				strings.EqualFold(k, "X-Luna-AI-Preview-Lock") {
				continue
			}
			req.Header[k] = append([]string(nil), vals...)
		}
		req.RemoteAddr = parent.Request.RemoteAddr
	}
	if bodyBytes != nil {
		req.Header.Set("Content-Type", "application/json")
		req.ContentLength = int64(len(bodyBytes))
	}
	if strings.TrimSpace(idempotencyKey) != "" {
		req.Header.Set("Idempotency-Key", strings.TrimSpace(idempotencyKey))
	}
	subGin.Request = req
	if len(params) > 0 {
		subGin.Params = append(gin.Params(nil), params...)
	}
	if parent != nil && parent.Keys != nil {
		subGin.Keys = maps.Clone(parent.Keys)
	}
	return subGin, rec
}

func errorFromRecorder(rec *httptest.ResponseRecorder, fallbackCode, fallbackMessage string) *ToolError {
	status := rec.Code
	if status < 400 {
		status = http.StatusBadRequest
	}
	raw := bytes.TrimSpace(rec.Body.Bytes())
	if len(raw) == 0 {
		return NewError(status, fallbackCode, fallbackMessage)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return NewError(status, fallbackCode, string(raw))
	}
	code, _ := parsed["code"].(string)
	if strings.TrimSpace(code) == "" {
		code = fallbackCode
	}
	msg, _ := parsed["message"].(string)
	if strings.TrimSpace(msg) == "" {
		msg = fallbackMessage
	}
	return &ToolError{
		Status:  status,
		Code:    code,
		Message: msg,
		RawBody: append(json.RawMessage(nil), raw...),
	}
}

func AsToolError(err error) (*ToolError, bool) {
	if err == nil {
		return nil, false
	}
	var te *ToolError
	if errors.As(err, &te) && te != nil {
		return te, true
	}
	return nil, false
}
