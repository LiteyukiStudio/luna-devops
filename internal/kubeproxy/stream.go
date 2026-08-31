package kubeproxy

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type RevalidateFunc func(context.Context, AccessContext) (AccessContext, error)

type StreamConfig struct {
	RevalidateInterval time.Duration
	WatchMaxDuration   time.Duration
	LogsMaxDuration    time.Duration
	UpgradeMaxDuration time.Duration
	IdleTimeout        time.Duration
}

func DefaultStreamConfig() StreamConfig {
	return StreamConfig{RevalidateInterval: 30 * time.Second, WatchMaxDuration: 30 * time.Minute, LogsMaxDuration: 2 * time.Hour, UpgradeMaxDuration: 2 * time.Hour, IdleTimeout: 15 * time.Minute}
}

var (
	ErrStreamLifetimeExpired   = errors.New("stream lifetime expired")
	ErrStreamCredentialExpired = errors.New("stream credential expired")
	ErrStreamAuthorizationLost = errors.New("stream authorization lost")
	ErrStreamIdle              = errors.New("stream idle timeout")
)

type streamActivityContextKey struct{}

type streamActivity struct {
	signal chan struct{}
}

func touchStream(ctx context.Context) {
	activity, _ := ctx.Value(streamActivityContextKey{}).(*streamActivity)
	if activity == nil {
		return
	}
	select {
	case activity.signal <- struct{}{}:
	default:
	}
}

type StreamController struct {
	Limiter    Limiter
	Config     StreamConfig
	Revalidate RevalidateFunc
}

func (controller StreamController) Open(parent context.Context, access AccessContext, class StreamClass) (context.Context, func(), error) {
	if controller.Limiter == nil || controller.Revalidate == nil {
		return nil, nil, Unavailable(CodeUnavailable, fmt.Errorf("stream authorization dependencies are unavailable"))
	}
	release, err := controller.Limiter.AcquireStream(parent, access, class)
	if err != nil {
		return nil, nil, err
	}
	config := controller.Config
	defaults := DefaultStreamConfig()
	if config.RevalidateInterval <= 0 {
		config.RevalidateInterval = defaults.RevalidateInterval
	}
	if config.IdleTimeout <= 0 {
		config.IdleTimeout = defaults.IdleTimeout
	}
	maxDuration := config.UpgradeMaxDuration
	switch class {
	case StreamWatch:
		maxDuration = config.WatchMaxDuration
	case StreamLogs:
		maxDuration = config.LogsMaxDuration
	}
	if maxDuration <= 0 {
		switch class {
		case StreamWatch:
			maxDuration = defaults.WatchMaxDuration
		case StreamLogs:
			maxDuration = defaults.LogsMaxDuration
		default:
			maxDuration = defaults.UpgradeMaxDuration
		}
	}
	ctx, cancel := context.WithCancelCause(parent)
	deadline := time.Now().Add(maxDuration)
	deadlineCause := ErrStreamLifetimeExpired
	if !access.ExpiresAt.IsZero() && access.ExpiresAt.Before(deadline) {
		deadline = access.ExpiresAt
		deadlineCause = ErrStreamCredentialExpired
	}
	activity := &streamActivity{signal: make(chan struct{}, 1)}
	ctx = context.WithValue(ctx, streamActivityContextKey{}, activity)
	go controller.monitor(ctx, cancel, access, deadline, deadlineCause, config.RevalidateInterval, config.IdleTimeout, activity)
	var once sync.Once
	closeStream := func() {
		once.Do(func() {
			cancel(context.Canceled)
			release()
		})
	}
	return ctx, closeStream, nil
}

func (controller StreamController) monitor(ctx context.Context, cancel context.CancelCauseFunc, access AccessContext, deadline time.Time, deadlineCause error, interval, idleTimeout time.Duration, activity *streamActivity) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	idle := time.NewTimer(idleTimeout)
	defer idle.Stop()
	current := access
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			cancel(deadlineCause)
			return
		case <-idle.C:
			cancel(ErrStreamIdle)
			return
		case <-activity.signal:
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(idleTimeout)
		case <-ticker.C:
			updated, err := controller.Revalidate(ctx, current)
			if err != nil {
				cancel(fmt.Errorf("%w: %v", ErrStreamAuthorizationLost, err))
				return
			}
			current = updated
		}
	}
}
