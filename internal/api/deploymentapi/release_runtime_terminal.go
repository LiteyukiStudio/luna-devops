package deploymentapi

import (
	"io"
	"sync"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/tools/remotecommand"
)

type releaseRuntimeExecInput struct {
	Command   string `json:"command"`
	Container string `json:"container"`
}

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
