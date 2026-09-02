package runtimeapi

import (
	"context"
	"encoding/json"
	"io"
	"sync"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/tools/remotecommand"
)

type runtimeTerminalClientMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type runtimeTerminalSizeQueue struct {
	ch chan remotecommand.TerminalSize
}

func newRuntimeTerminalSizeQueue() *runtimeTerminalSizeQueue {
	return &runtimeTerminalSizeQueue{ch: make(chan remotecommand.TerminalSize, 8)}
}

func (q *runtimeTerminalSizeQueue) Push(cols uint16, rows uint16) {
	if q == nil || cols == 0 || rows == 0 {
		return
	}
	size := remotecommand.TerminalSize{Width: cols, Height: rows}
	select {
	case q.ch <- size:
	default:
		select {
		case <-q.ch:
		default:
		}
		q.ch <- size
	}
}

func (q *runtimeTerminalSizeQueue) Next() *remotecommand.TerminalSize {
	if q == nil {
		return nil
	}
	size, ok := <-q.ch
	if !ok {
		return nil
	}
	return &size
}

type runtimeTerminalWebSocketWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *runtimeTerminalWebSocketWriter) Write(data []byte) (int, error) {
	if w == nil || w.conn == nil {
		return 0, io.ErrClosedPipe
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (h *Handlers) readRuntimeTerminalMessages(conn *websocket.Conn, stdin *io.PipeWriter, sizeQueue *runtimeTerminalSizeQueue, cancel context.CancelFunc) {
	defer cancel()
	defer stdin.Close()
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		data, isInput := runtimeTerminalInputPayload(messageType, data, sizeQueue)
		if !isInput {
			continue
		}
		if _, err := stdin.Write(data); err != nil {
			return
		}
	}
}

func runtimeTerminalInputPayload(messageType int, data []byte, sizeQueue *runtimeTerminalSizeQueue) ([]byte, bool) {
	if len(data) == 0 || (messageType != websocket.TextMessage && messageType != websocket.BinaryMessage) {
		return nil, false
	}
	if messageType == websocket.TextMessage {
		var message runtimeTerminalClientMessage
		if err := json.Unmarshal(data, &message); err == nil && message.Type == "resize" {
			sizeQueue.Push(message.Cols, message.Rows)
			return nil, false
		}
	}
	return data, true
}
