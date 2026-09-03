package transport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/tools/remotecommand"
)

func TestRuntimeTerminalSizeQueueValidatesAndCloses(t *testing.T) {
	queue := NewRuntimeTerminalSizeQueue()
	for _, size := range [][2]int{{0, 24}, {80, 0}, {-1, 24}, {80, 65536}} {
		if queue.Push(size[0], size[1]) {
			t.Fatalf("invalid terminal size %v was accepted", size)
		}
	}
	if !queue.Push(65535, 1) {
		t.Fatal("valid maximum terminal size was rejected")
	}
	size := queue.Next()
	if size == nil || size.Width != 65535 || size.Height != 1 {
		t.Fatalf("terminal size = %#v, want 65535x1", size)
	}

	next := make(chan *remotecommand.TerminalSize, 1)
	go func() { next <- queue.Next() }()
	queue.Close()
	select {
	case value := <-next:
		if value != nil {
			t.Fatalf("closed queue returned %#v, want nil", value)
		}
	case <-time.After(time.Second):
		t.Fatal("closing the terminal size queue did not unblock Next")
	}
	if queue.Push(80, 24) {
		t.Fatal("closed terminal size queue accepted another size")
	}
	queue.Close()
}

func TestRuntimeTerminalResizeControlIsStrict(t *testing.T) {
	queue := NewRuntimeTerminalSizeQueue()
	defer queue.Close()
	if !applyRuntimeTerminalResize([]byte(`{"type":"resize","cols":120,"rows":40}`), queue) {
		t.Fatal("valid resize control was rejected")
	}
	size := queue.Next()
	if size == nil || size.Width != 120 || size.Height != 40 {
		t.Fatalf("resize control produced %#v", size)
	}
	for _, payload := range []string{
		`{"type":"resize","cols":0,"rows":24}`,
		`{"type":"resize","cols":80,"rows":65536}`,
		`{"type":"input","cols":80,"rows":24}`,
		`{"type":"resize","cols":80,"rows":24,"extra":true}`,
		`{"type":"resize","cols":80,"rows":24} trailing`,
	} {
		if applyRuntimeTerminalResize([]byte(payload), queue) {
			t.Fatalf("invalid resize control %q was accepted", payload)
		}
	}
}

func TestRuntimeTerminalSubprotocolMustBeRequested(t *testing.T) {
	for _, test := range []struct {
		header string
		want   bool
	}{
		{header: "", want: false},
		{header: "other.protocol", want: false},
		{header: "other.protocol, " + RuntimeTerminalWebSocketSubprotocol, want: true},
	} {
		request := httptest.NewRequest(http.MethodGet, "/terminal", nil)
		request.Header.Set("Sec-WebSocket-Protocol", test.header)
		if got := RuntimeTerminalSubprotocolRequested(request); got != test.want {
			t.Fatalf("subprotocol header %q accepted = %t, want %t", test.header, got, test.want)
		}
	}
}

func TestRuntimeTerminalWebSocketUsesVersionedBinaryProtocol(t *testing.T) {
	serverErr := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upgrader := RuntimeTerminalUpgrader(func(string) bool { return true })
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		if conn.Subprotocol() != RuntimeTerminalWebSocketSubprotocol {
			serverErr <- errors.New("terminal subprotocol was not negotiated")
			return
		}
		socket := NewRuntimeTerminalWebSocket(conn)
		if _, err := socket.Write([]byte{0x00, 0xff, 0x1b}); err != nil {
			serverErr <- err
			return
		}
		if end := FinishRuntimeTerminal(socket, 42, nil, nil, false, nil); end != RuntimeTerminalEndExited {
			serverErr <- errors.New("terminal did not finish as a remote exit")
			return
		}
		serverErr <- nil
	}))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	dialer := websocket.Dialer{Subprotocols: []string{RuntimeTerminalWebSocketSubprotocol}}
	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if conn.Subprotocol() != RuntimeTerminalWebSocketSubprotocol {
		t.Fatalf("negotiated subprotocol = %q", conn.Subprotocol())
	}

	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if messageType != websocket.BinaryMessage || !bytes.Equal(payload, []byte{0x00, 0xff, 0x1b}) {
		t.Fatalf("stdout frame = type %d payload %v", messageType, payload)
	}
	messageType, payload, err = conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if messageType != websocket.TextMessage || string(payload) != `{"type":"exit","code":42}` {
		t.Fatalf("exit frame = type %d payload %q", messageType, payload)
	}
	_, _, err = conn.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) || closeErr.Code != websocket.CloseNormalClosure {
		t.Fatalf("terminal close = %v, want code 1000", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeTerminalWebSocketUsesSemanticFailureCloseCodes(t *testing.T) {
	tests := []struct {
		name          string
		streamErr     error
		contextErr    error
		revoked       bool
		wantEnd       RuntimeTerminalEnd
		wantCloseCode int
	}{
		{
			name:          "stream failure",
			streamErr:     errors.New("upstream stream failed"),
			wantEnd:       RuntimeTerminalEndInternalError,
			wantCloseCode: websocket.CloseInternalServerErr,
		},
		{
			name:          "authorization revoked",
			streamErr:     context.Canceled,
			contextErr:    context.Canceled,
			revoked:       true,
			wantEnd:       RuntimeTerminalEndAuthorizationLost,
			wantCloseCode: websocket.ClosePolicyViolation,
		},
		{
			name:          "authorization expired",
			streamErr:     context.DeadlineExceeded,
			contextErr:    context.DeadlineExceeded,
			wantEnd:       RuntimeTerminalEndAuthorizationExpiry,
			wantCloseCode: websocket.ClosePolicyViolation,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			serverEnd := make(chan RuntimeTerminalEnd, 1)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				upgrader := RuntimeTerminalUpgrader(func(string) bool { return true })
				conn, err := upgrader.Upgrade(writer, request, nil)
				if err != nil {
					serverEnd <- "upgrade_failed"
					return
				}
				defer conn.Close()
				socket := NewRuntimeTerminalWebSocket(conn)
				serverEnd <- FinishRuntimeTerminal(socket, 0, test.streamErr, test.contextErr, test.revoked, nil)
			}))
			defer server.Close()

			url := "ws" + strings.TrimPrefix(server.URL, "http")
			dialer := websocket.Dialer{Subprotocols: []string{RuntimeTerminalWebSocketSubprotocol}}
			conn, _, err := dialer.Dial(url, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_, _, err = conn.ReadMessage()
			var closeErr *websocket.CloseError
			if !errors.As(err, &closeErr) || closeErr.Code != test.wantCloseCode {
				t.Fatalf("terminal close = %v, want code %d", err, test.wantCloseCode)
			}
			if end := <-serverEnd; end != test.wantEnd {
				t.Fatalf("terminal end = %q, want %q", end, test.wantEnd)
			}
		})
	}
}

func TestRuntimeTerminalInputRequiresBinaryFrames(t *testing.T) {
	type pumpResult struct {
		input []byte
		size  [2]uint16
		err   error
	}
	result := make(chan pumpResult, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upgrader := RuntimeTerminalUpgrader(func(string) bool { return true })
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			result <- pumpResult{err: err}
			return
		}
		defer conn.Close()
		_, cancel := context.WithCancel(context.Background())
		defer cancel()
		stdinReader, stdinWriter := io.Pipe()
		defer stdinReader.Close()
		queue := NewRuntimeTerminalSizeQueue()
		socket := NewRuntimeTerminalWebSocket(conn)
		inputDone := socket.PumpInput(stdinWriter, queue, cancel)

		input := make([]byte, 5)
		if _, err := io.ReadFull(stdinReader, input); err != nil {
			result <- pumpResult{err: err}
			return
		}
		size := queue.Next()
		pumpErr := <-inputDone
		if size == nil {
			result <- pumpResult{err: errors.New("resize control did not reach the size queue")}
			return
		}
		result <- pumpResult{input: input, size: [2]uint16{size.Width, size.Height}, err: pumpErr}
	}))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	dialer := websocket.Dialer{Subprotocols: []string{RuntimeTerminalWebSocketSubprotocol}}
	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	input := []byte{0xe4, 0xb8, 0xad, 0x1b, 0x03}
	if err := conn.WriteMessage(websocket.BinaryMessage, input); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"resize","cols":132,"rows":43}`)); err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("text is not stdin")); err != nil {
		t.Fatal(err)
	}
	_, _, err = conn.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) || closeErr.Code != websocket.CloseProtocolError {
		t.Fatalf("invalid text frame close = %v, want code 1002", err)
	}
	pump := <-result
	if !bytes.Equal(pump.input, input) || pump.size != [2]uint16{132, 43} {
		t.Fatalf("pump result = input %v size %v", pump.input, pump.size)
	}
	if !errors.Is(pump.err, errRuntimeTerminalProtocol) {
		t.Fatalf("pump error = %v, want protocol error", pump.err)
	}
}
