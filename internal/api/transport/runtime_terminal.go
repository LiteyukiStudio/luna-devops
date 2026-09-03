package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/tools/remotecommand"
)

const RuntimeTerminalWebSocketSubprotocol = "luna.devops.terminal.v1"

const (
	RuntimeTerminalEndExited              RuntimeTerminalEnd = "exited"
	RuntimeTerminalEndClientDisconnected  RuntimeTerminalEnd = "client_disconnected"
	RuntimeTerminalEndProtocolError       RuntimeTerminalEnd = "protocol_error"
	RuntimeTerminalEndAuthorizationLost   RuntimeTerminalEnd = "authorization_lost"
	RuntimeTerminalEndAuthorizationExpiry RuntimeTerminalEnd = "authorization_expired"
	RuntimeTerminalEndInternalError       RuntimeTerminalEnd = "internal_error"
)

var errRuntimeTerminalProtocol = errors.New("invalid runtime terminal protocol message")

type RuntimeTerminalEnd string

type runtimeTerminalClientMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

type runtimeTerminalExitMessage struct {
	Type string `json:"type"`
	Code int    `json:"code"`
}

type RuntimeTerminalSizeQueue struct {
	mu     sync.Mutex
	ch     chan remotecommand.TerminalSize
	closed bool
}

func NewRuntimeTerminalSizeQueue() *RuntimeTerminalSizeQueue {
	return &RuntimeTerminalSizeQueue{ch: make(chan remotecommand.TerminalSize, 8)}
}

func (q *RuntimeTerminalSizeQueue) Push(cols, rows int) bool {
	if q == nil || cols < 1 || cols > 65535 || rows < 1 || rows > 65535 {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	size := remotecommand.TerminalSize{Width: uint16(cols), Height: uint16(rows)}
	select {
	case q.ch <- size:
	default:
		select {
		case <-q.ch:
		default:
		}
		q.ch <- size
	}
	return true
}

func (q *RuntimeTerminalSizeQueue) Next() *remotecommand.TerminalSize {
	if q == nil {
		return nil
	}
	size, ok := <-q.ch
	if !ok {
		return nil
	}
	return &size
}

func (q *RuntimeTerminalSizeQueue) Close() {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	for len(q.ch) > 0 {
		<-q.ch
	}
	close(q.ch)
}

type RuntimeTerminalWebSocket struct {
	conn   *websocket.Conn
	mu     sync.Mutex
	closed bool
}

func NewRuntimeTerminalWebSocket(conn *websocket.Conn) *RuntimeTerminalWebSocket {
	return &RuntimeTerminalWebSocket{conn: conn}
}

func RuntimeTerminalUpgrader(allowedOrigin func(string) bool) websocket.Upgrader {
	return websocket.Upgrader{
		Subprotocols: []string{RuntimeTerminalWebSocketSubprotocol},
		CheckOrigin: func(request *http.Request) bool {
			origin := strings.TrimSpace(request.Header.Get("Origin"))
			return origin == "" || allowedOrigin == nil || allowedOrigin(origin)
		},
	}
}

func RuntimeTerminalSubprotocolRequested(request *http.Request) bool {
	if request == nil {
		return false
	}
	for _, protocol := range websocket.Subprotocols(request) {
		if protocol == RuntimeTerminalWebSocketSubprotocol {
			return true
		}
	}
	return false
}

func (socket *RuntimeTerminalWebSocket) Write(data []byte) (int, error) {
	if socket == nil || socket.conn == nil {
		return 0, io.ErrClosedPipe
	}
	socket.mu.Lock()
	defer socket.mu.Unlock()
	if socket.closed {
		return 0, io.ErrClosedPipe
	}
	if err := socket.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (socket *RuntimeTerminalWebSocket) SendExit(code int) error {
	if socket == nil || socket.conn == nil {
		return io.ErrClosedPipe
	}
	socket.mu.Lock()
	defer socket.mu.Unlock()
	if socket.closed {
		return io.ErrClosedPipe
	}
	payload, err := json.Marshal(runtimeTerminalExitMessage{Type: "exit", Code: code})
	if err != nil {
		return err
	}
	return socket.conn.WriteMessage(websocket.TextMessage, payload)
}

func (socket *RuntimeTerminalWebSocket) Close(code int, reason string) error {
	if socket == nil || socket.conn == nil {
		return io.ErrClosedPipe
	}
	socket.mu.Lock()
	defer socket.mu.Unlock()
	if socket.closed {
		return nil
	}
	socket.closed = true
	return socket.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(code, reason),
		time.Now().Add(time.Second),
	)
}

func (socket *RuntimeTerminalWebSocket) CloseAuthorizationRevoked() error {
	return socket.Close(websocket.ClosePolicyViolation, "terminal authorization revoked")
}

func (socket *RuntimeTerminalWebSocket) PumpInput(
	stdin *io.PipeWriter,
	sizeQueue *RuntimeTerminalSizeQueue,
	cancel context.CancelFunc,
) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		defer cancel()
		defer stdin.Close()
		defer sizeQueue.Close()
		for {
			messageType, data, err := socket.conn.ReadMessage()
			if err != nil {
				done <- err
				return
			}
			switch messageType {
			case websocket.BinaryMessage:
				if len(data) == 0 {
					continue
				}
				if _, err := stdin.Write(data); err != nil {
					done <- err
					return
				}
			case websocket.TextMessage:
				if !applyRuntimeTerminalResize(data, sizeQueue) {
					_ = socket.Close(websocket.CloseProtocolError, "invalid terminal control message")
					done <- errRuntimeTerminalProtocol
					return
				}
			default:
				_ = socket.Close(websocket.CloseProtocolError, "unsupported terminal frame")
				done <- errRuntimeTerminalProtocol
				return
			}
		}
	}()
	return done
}

func FinishRuntimeTerminal(
	socket *RuntimeTerminalWebSocket,
	exitCode int,
	streamErr error,
	contextErr error,
	authorizationRevoked bool,
	inputDone <-chan error,
) RuntimeTerminalEnd {
	var inputErr error
	select {
	case inputErr = <-inputDone:
	default:
	}
	if errors.Is(inputErr, errRuntimeTerminalProtocol) {
		return RuntimeTerminalEndProtocolError
	}
	if authorizationRevoked {
		_ = socket.CloseAuthorizationRevoked()
		return RuntimeTerminalEndAuthorizationLost
	}
	if errors.Is(contextErr, context.DeadlineExceeded) {
		_ = socket.Close(websocket.ClosePolicyViolation, "terminal authorization expired")
		return RuntimeTerminalEndAuthorizationExpiry
	}
	if inputErr != nil {
		return RuntimeTerminalEndClientDisconnected
	}
	if streamErr != nil {
		if errors.Is(contextErr, context.Canceled) {
			return RuntimeTerminalEndClientDisconnected
		}
		_ = socket.Close(websocket.CloseInternalServerErr, "terminal stream failed")
		return RuntimeTerminalEndInternalError
	}
	if contextErr != nil {
		return RuntimeTerminalEndClientDisconnected
	}
	if err := socket.SendExit(exitCode); err != nil {
		return RuntimeTerminalEndClientDisconnected
	}
	if err := socket.Close(websocket.CloseNormalClosure, ""); err != nil {
		return RuntimeTerminalEndClientDisconnected
	}
	return RuntimeTerminalEndExited
}

func applyRuntimeTerminalResize(data []byte, sizeQueue *RuntimeTerminalSizeQueue) bool {
	var message runtimeTerminalClientMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&message); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return false
	}
	return message.Type == "resize" && sizeQueue.Push(message.Cols, message.Rows)
}
