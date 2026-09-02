package notificationapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	inboxStreamPollInterval      = 15 * time.Second
	inboxStreamHeartbeatInterval = 25 * time.Second
)

type InboxChange struct {
	MessageID string
}

type InboxChangeBroker struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan InboxChange]struct{}
}

var defaultInboxBroker = NewInboxChangeBroker()

func DefaultInboxBroker() *InboxChangeBroker {
	return defaultInboxBroker
}

func NewInboxChangeBroker() *InboxChangeBroker {
	return &InboxChangeBroker{subscribers: make(map[string]map[chan InboxChange]struct{})}
}

func (b *InboxChangeBroker) Subscribe(userID string) (<-chan InboxChange, func()) {
	changes := make(chan InboxChange, 1)
	b.mu.Lock()
	if b.subscribers[userID] == nil {
		b.subscribers[userID] = make(map[chan InboxChange]struct{})
	}
	b.subscribers[userID][changes] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	return changes, func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subscribers[userID], changes)
			if len(b.subscribers[userID]) == 0 {
				delete(b.subscribers, userID)
			}
			b.mu.Unlock()
		})
	}
}

func (b *InboxChangeBroker) Notify(userID, messageID string) {
	b.mu.RLock()
	subscribers := make([]chan InboxChange, 0, len(b.subscribers[userID]))
	for subscriber := range b.subscribers[userID] {
		subscribers = append(subscribers, subscriber)
	}
	b.mu.RUnlock()

	change := InboxChange{MessageID: messageID}
	for _, subscriber := range subscribers {
		select {
		case subscriber <- change:
		default:
		}
	}
}

type InboxChangedEvent struct {
	UnreadCount int64  `json:"unreadCount"`
	MessageID   string `json:"messageId,omitempty"`
}

func (h *Handler) StreamInboxChanges(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	unreadCount, err := h.inboxService().UnreadCount(ctx.Request.Context(), user.ID)
	if err != nil {
		WriteInboxError(ctx, err)
		return
	}
	flusher, ok := ctx.Writer.(http.Flusher)
	if !ok {
		writeErrorCode(ctx, http.StatusInternalServerError, "inbox.stream_unsupported", "streaming is unsupported")
		return
	}

	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache, no-store")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")
	ctx.Status(http.StatusOK)

	changes, unsubscribe := defaultInboxBroker.Subscribe(user.ID)
	defer unsubscribe()

	if err := WriteInboxChangedEvent(ctx.Writer, InboxChangedEvent{UnreadCount: unreadCount}); err != nil {
		return
	}
	flusher.Flush()

	pollTicker := time.NewTicker(inboxStreamPollInterval)
	heartbeatTicker := time.NewTicker(inboxStreamHeartbeatInterval)
	defer pollTicker.Stop()
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-ctx.Request.Context().Done():
			return
		case change := <-changes:
			count, countErr := h.inboxService().UnreadCount(ctx.Request.Context(), user.ID)
			if countErr != nil {
				return
			}
			unreadCount = count
			if err := WriteInboxChangedEvent(ctx.Writer, InboxChangedEvent{UnreadCount: count, MessageID: change.MessageID}); err != nil {
				return
			}
			flusher.Flush()
		case <-pollTicker.C:
			count, countErr := h.inboxService().UnreadCount(ctx.Request.Context(), user.ID)
			if countErr != nil {
				return
			}
			if count == unreadCount {
				continue
			}
			unreadCount = count
			if err := WriteInboxChangedEvent(ctx.Writer, InboxChangedEvent{UnreadCount: count}); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeatTicker.C:
			if _, err := io.WriteString(ctx.Writer, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func WriteInboxChangedEvent(writer io.Writer, event InboxChangedEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "event: inbox.changed\ndata: %s\n\n", payload)
	return err
}
