package kubeproxy

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStreamControllerCancelsWhenRevalidationFails(t *testing.T) {
	limiter := NewLocalLimiter(DefaultLimiterConfig())
	controller := StreamController{
		Limiter: limiter,
		Config:  StreamConfig{RevalidateInterval: time.Millisecond, WatchMaxDuration: time.Minute},
		Revalidate: func(_ context.Context, _ AccessContext) (AccessContext, error) {
			return AccessContext{}, errors.New("revoked")
		},
	}
	ctx, closeStream, err := controller.Open(t.Context(), baseAccess(), StreamWatch)
	if err != nil {
		t.Fatal(err)
	}
	defer closeStream()
	select {
	case <-ctx.Done():
		if !errors.Is(context.Cause(ctx), ErrStreamAuthorizationLost) {
			t.Fatalf("unexpected cancellation cause: %v", context.Cause(ctx))
		}
	case <-time.After(time.Second):
		t.Fatal("stream was not cancelled after revocation")
	}
}

func TestStreamControllerCancelsIdleStream(t *testing.T) {
	controller := StreamController{
		Limiter: NewLocalLimiter(DefaultLimiterConfig()),
		Config:  StreamConfig{RevalidateInterval: time.Minute, WatchMaxDuration: time.Minute, IdleTimeout: 5 * time.Millisecond},
		Revalidate: func(_ context.Context, access AccessContext) (AccessContext, error) {
			return access, nil
		},
	}
	ctx, closeStream, err := controller.Open(t.Context(), baseAccess(), StreamWatch)
	if err != nil {
		t.Fatal(err)
	}
	defer closeStream()
	select {
	case <-ctx.Done():
		if !errors.Is(context.Cause(ctx), ErrStreamIdle) {
			t.Fatalf("unexpected idle cancellation cause: %v", context.Cause(ctx))
		}
	case <-time.After(time.Second):
		t.Fatal("idle stream was not cancelled")
	}
}
