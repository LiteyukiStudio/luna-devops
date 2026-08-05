package runtimecommand

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
)

const (
	defaultIdleTTL     = 5 * time.Minute
	defaultAbsoluteTTL = 30 * time.Minute
	defaultOutputLimit = 256 * 1024
	defaultCommandTTL  = 30 * time.Second
	cleanupInterval    = 15 * time.Second
)

var (
	ErrNotFound        = errors.New("runtime command session not found")
	ErrOwnerMismatch   = errors.New("runtime command session belongs to another API instance")
	ErrBindingMismatch = errors.New("runtime command session binding mismatch")
	ErrExpired         = errors.New("runtime command session expired")
	ErrClosed          = errors.New("runtime command session closed")
	ErrCommandTooLong  = errors.New("runtime command is too long")
)

// Binding is the immutable authority and resource boundary of a shell session.
type Binding struct {
	UserID             string
	SubjectID          string
	AgentRunID         string
	ProjectID          string
	ApplicationID      string
	ReleaseID          string
	DeploymentTargetID string
	Container          string
}

func (binding Binding) Equal(other Binding) bool {
	return binding.UserID == other.UserID && binding.SubjectID == other.SubjectID &&
		binding.AgentRunID == other.AgentRunID && binding.ProjectID == other.ProjectID &&
		binding.ApplicationID == other.ApplicationID && binding.ReleaseID == other.ReleaseID &&
		binding.DeploymentTargetID == other.DeploymentTargetID &&
		(strings.TrimSpace(other.Container) == "" || binding.Container == other.Container)
}

type Snapshot struct {
	ID          string    `json:"sessionId"`
	Container   string    `json:"container,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	LastActive  time.Time `json:"lastActiveAt"`
	IdleExpires time.Time `json:"idleExpiresAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

type Result struct {
	Stdout    string `json:"stdout"`
	ExitCode  int    `json:"exitCode"`
	Truncated bool   `json:"truncated"`
	Duration  int64  `json:"durationMs"`
}

// StartFunc connects a persistent non-TTY shell to the supplied streams and
// blocks until the shell exits. The broker owns both streams and their context.
type StartFunc func(context.Context, io.Reader, io.Writer) error

type Options struct {
	InstanceID  string
	IdleTTL     time.Duration
	AbsoluteTTL time.Duration
	OutputLimit int
	CommandTTL  time.Duration
	Now         func() time.Time
}

type Broker struct {
	mu       sync.RWMutex
	sessions map[string]*session
	opts     Options
	ownerID  string
	stop     chan struct{}
	started  sync.Once
	closed   sync.Once
}

type session struct {
	id      string
	binding Binding
	created time.Time

	ctx    context.Context
	cancel context.CancelFunc
	stdin  *io.PipeWriter

	commandMu sync.Mutex
	mu        sync.Mutex
	last      time.Time
	output    []byte
	truncated bool
	limit     int
	notify    chan struct{}
	closed    bool
	err       error
}

func NewBroker(options Options) *Broker {
	if options.IdleTTL <= 0 {
		options.IdleTTL = defaultIdleTTL
	}
	if options.AbsoluteTTL <= 0 {
		options.AbsoluteTTL = defaultAbsoluteTTL
	}
	if options.OutputLimit <= 0 {
		options.OutputLimit = defaultOutputLimit
	}
	if options.CommandTTL <= 0 {
		options.CommandTTL = defaultCommandTTL
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	ownerID := normalizeOwnerID(options.InstanceID)
	if ownerID == "" {
		generated, _ := randomID("", 4)
		ownerID = generated
	}
	broker := &Broker{sessions: map[string]*session{}, opts: options, ownerID: ownerID, stop: make(chan struct{})}
	return broker
}

func (broker *Broker) Create(ctx context.Context, binding Binding, start StartFunc) (Snapshot, error) {
	if start == nil || strings.TrimSpace(binding.UserID) == "" || strings.TrimSpace(binding.SubjectID) == "" ||
		strings.TrimSpace(binding.ProjectID) == "" || strings.TrimSpace(binding.ReleaseID) == "" ||
		strings.TrimSpace(binding.DeploymentTargetID) == "" {
		return Snapshot{}, ErrBindingMismatch
	}
	broker.started.Do(func() { go broker.reapExpired() })
	now := broker.opts.Now()
	random, err := randomID("", 18)
	if err != nil {
		return Snapshot{}, err
	}
	id := "rtcs_" + broker.ownerID + "_" + random
	lifecycleCtx, cancel := context.WithDeadline(context.WithoutCancel(ctx), now.Add(broker.opts.AbsoluteTTL))
	stdinReader, stdinWriter := io.Pipe()
	item := &session{
		id: id, binding: binding, created: now, last: now,
		ctx: lifecycleCtx, cancel: cancel, stdin: stdinWriter, notify: make(chan struct{}), limit: broker.opts.OutputLimit,
	}
	broker.mu.Lock()
	broker.sessions[id] = item
	broker.mu.Unlock()

	go func() {
		runCtx, end := telemetry.StartOperation(lifecycleCtx, "runtime_command_session", "lifecycle",
			attribute.String("runtime.session.kind", "agent_shell"),
		)
		runErr := start(runCtx, stdinReader, sessionWriter{session: item})
		_ = stdinReader.Close()
		item.finish(runErr)
		end(runErr)
	}()
	telemetry.Logger().InfoContext(ctx, "runtime command session created",
		slog.String("event.name", "runtime.command_session.created"),
		slog.String("runtime.session_id", id),
	)
	return broker.snapshot(item), nil
}

func (broker *Broker) Execute(ctx context.Context, id string, binding Binding, command string) (result Result, err error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return Result{}, ErrCommandTooLong
	}
	if len(command) > 16*1024 {
		return Result{}, ErrCommandTooLong
	}
	item, err := broker.boundSession(id, binding)
	if err != nil {
		return Result{}, err
	}
	item.commandMu.Lock()
	defer item.commandMu.Unlock()

	now := broker.opts.Now()
	if err := broker.ensureActive(item, now); err != nil {
		return Result{}, err
	}
	token, err := randomID("", 12)
	if err != nil {
		return Result{}, err
	}
	begin := "\x1eLUNA_BEGIN_" + token + "\x1f\n"
	endMarker := "\x1eLUNA_END_" + token + ":"
	payload := "printf '" + shellSingleQuote(begin) + "'; eval '" + shellSingleQuote(command) + "'; __luna_status=$?; printf '\\n" + shellSingleQuote(endMarker) + "%s\\037\\n' \"$__luna_status\"\n"

	item.resetOutput(now)
	started := time.Now()
	opCtx, finish := telemetry.StartOperation(ctx, "runtime_command_session", "execute",
		attribute.String("runtime.session.kind", "agent_shell"),
	)
	defer func() { finish(err) }()
	commandCtx, cancel := context.WithTimeout(opCtx, broker.opts.CommandTTL)
	defer cancel()
	if _, err = io.WriteString(item.stdin, payload); err != nil {
		item.finish(err)
		return Result{}, ErrClosed
	}
	output, truncated, waitErr := item.waitForMarker(commandCtx, begin, endMarker)
	if waitErr != nil {
		broker.remove(id)
		item.finish(waitErr)
		return Result{}, waitErr
	}
	stdout, exitCode, parseErr := parseCommandOutput(output, begin, endMarker)
	if parseErr != nil {
		return Result{}, parseErr
	}
	item.touch(broker.opts.Now())
	result = Result{Stdout: stdout, ExitCode: exitCode, Truncated: truncated, Duration: time.Since(started).Milliseconds()}
	telemetry.Logger().InfoContext(opCtx, "runtime command session command completed",
		slog.String("event.name", "runtime.command_session.command.completed"),
		slog.String("runtime.session_id", id),
		slog.Int("exit_code", exitCode),
		slog.Int64("duration_ms", result.Duration),
	)
	return result, nil
}

func (broker *Broker) Close(ctx context.Context, id string, binding Binding) error {
	item, err := broker.boundSession(id, binding)
	if err != nil {
		return err
	}
	broker.remove(id)
	item.finish(nil)
	telemetry.Logger().InfoContext(ctx, "runtime command session closed",
		slog.String("event.name", "runtime.command_session.closed"),
		slog.String("runtime.session_id", id),
	)
	return nil
}

func (broker *Broker) Shutdown(ctx context.Context) error {
	broker.closed.Do(func() { close(broker.stop) })
	broker.mu.Lock()
	items := make([]*session, 0, len(broker.sessions))
	for id, item := range broker.sessions {
		items = append(items, item)
		delete(broker.sessions, id)
	}
	broker.mu.Unlock()
	for _, item := range items {
		item.finish(context.Canceled)
	}
	return ctx.Err()
}

func (broker *Broker) boundSession(id string, binding Binding) (*session, error) {
	id = strings.TrimSpace(id)
	if owner := ownerFromSessionID(id); owner != "" && owner != broker.ownerID {
		return nil, ErrOwnerMismatch
	}
	broker.mu.RLock()
	item := broker.sessions[id]
	broker.mu.RUnlock()
	if item == nil {
		return nil, ErrNotFound
	}
	if !item.binding.Equal(binding) {
		return nil, ErrBindingMismatch
	}
	return item, nil
}

func (broker *Broker) ensureActive(item *session, now time.Time) error {
	item.mu.Lock()
	defer item.mu.Unlock()
	if item.closed {
		return ErrClosed
	}
	if !item.created.Add(broker.opts.AbsoluteTTL).After(now) || !item.last.Add(broker.opts.IdleTTL).After(now) {
		return ErrExpired
	}
	return nil
}

func (broker *Broker) snapshot(item *session) Snapshot {
	item.mu.Lock()
	defer item.mu.Unlock()
	return Snapshot{ID: item.id, Container: item.binding.Container, CreatedAt: item.created, LastActive: item.last,
		IdleExpires: item.last.Add(broker.opts.IdleTTL), ExpiresAt: item.created.Add(broker.opts.AbsoluteTTL)}
}

func (broker *Broker) reapExpired() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-broker.stop:
			return
		case <-ticker.C:
			now := broker.opts.Now()
			broker.mu.RLock()
			items := make([]*session, 0, len(broker.sessions))
			for _, item := range broker.sessions {
				items = append(items, item)
			}
			broker.mu.RUnlock()
			for _, item := range items {
				if broker.ensureActive(item, now) == nil {
					continue
				}
				broker.remove(item.id)
				item.finish(ErrExpired)
			}
		}
	}
}

func (broker *Broker) remove(id string) {
	broker.mu.Lock()
	delete(broker.sessions, id)
	broker.mu.Unlock()
}

func (item *session) resetOutput(now time.Time) {
	item.mu.Lock()
	item.output = item.output[:0]
	item.truncated = false
	item.last = now
	item.mu.Unlock()
}

func (item *session) touch(now time.Time) {
	item.mu.Lock()
	item.last = now
	item.mu.Unlock()
}

func (item *session) finish(err error) {
	item.mu.Lock()
	if item.closed {
		item.mu.Unlock()
		return
	}
	item.closed = true
	item.err = err
	close(item.notify)
	item.mu.Unlock()
	item.cancel()
	_ = item.stdin.Close()
}

func (item *session) appendOutput(data []byte, limit int) {
	item.mu.Lock()
	if len(item.output)+len(data) > limit {
		item.truncated = true
		combined := append(append([]byte(nil), item.output...), data...)
		if len(combined) > limit {
			headSize := 512
			if headSize > limit/2 {
				headSize = limit / 2
			}
			tailSize := limit - headSize
			combined = append(append([]byte(nil), combined[:headSize]...), combined[len(combined)-tailSize:]...)
		}
		item.output = combined
	} else {
		item.output = append(item.output, data...)
	}
	close(item.notify)
	item.notify = make(chan struct{})
	item.mu.Unlock()
}

func (item *session) waitForMarker(ctx context.Context, begin, end string) ([]byte, bool, error) {
	for {
		item.mu.Lock()
		output := append([]byte(nil), item.output...)
		truncated := item.truncated
		notify := item.notify
		closed, closeErr := item.closed, item.err
		item.mu.Unlock()
		if hasCompleteCommandMarkers(output, begin, end) {
			return output, truncated, nil
		}
		if closed {
			if closeErr != nil {
				return nil, truncated, closeErr
			}
			return nil, truncated, ErrClosed
		}
		select {
		case <-ctx.Done():
			return nil, truncated, ctx.Err()
		case <-notify:
		}
	}
}

func hasCompleteCommandMarkers(output []byte, begin, end string) bool {
	value := string(output)
	beginIndex := strings.Index(value, begin)
	endIndex := strings.LastIndex(value, end)
	if beginIndex < 0 || endIndex < beginIndex {
		return false
	}
	return strings.Contains(value[endIndex+len(end):], "\x1f")
}

type sessionWriter struct{ session *session }

func (writer sessionWriter) Write(data []byte) (int, error) {
	if writer.session == nil {
		return 0, io.ErrClosedPipe
	}
	writer.session.appendOutput(data, writer.session.limit)
	return len(data), nil
}

func parseCommandOutput(output []byte, begin, end string) (string, int, error) {
	value := string(output)
	beginIndex := strings.Index(value, begin)
	endIndex := strings.LastIndex(value, end)
	if beginIndex < 0 || endIndex < 0 || endIndex < beginIndex {
		return "", 0, fmt.Errorf("runtime command markers are incomplete")
	}
	statusStart := endIndex + len(end)
	statusEnd := strings.Index(value[statusStart:], "\x1f")
	if statusEnd < 0 {
		return "", 0, fmt.Errorf("runtime command status marker is incomplete")
	}
	var exitCode int
	if _, err := fmt.Sscanf(value[statusStart:statusStart+statusEnd], "%d", &exitCode); err != nil {
		return "", 0, fmt.Errorf("parse runtime command exit status: %w", err)
	}
	stdout := value[beginIndex+len(begin) : endIndex]
	stdout = strings.TrimSuffix(stdout, "\n")
	return stdout, exitCode, nil
}

func shellSingleQuote(value string) string {
	return strings.ReplaceAll(value, "'", "'\\''")
}

func randomID(prefix string, bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(buffer), nil
}

func normalizeOwnerID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var result strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			result.WriteRune(char)
		}
		if result.Len() == 8 {
			break
		}
	}
	return result.String()
}

func ownerFromSessionID(id string) string {
	parts := strings.Split(id, "_")
	if len(parts) != 3 || parts[0] != "rtcs" {
		return ""
	}
	return parts[1]
}
