package volumeapi

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

func TestVolumeTransferDeadlineReaderInterruptsBlockedRead(t *testing.T) {
	controller := newDeadlineControllerStub()
	reader := &volumeTransferDeadlineReader{
		reader: &deadlineBlockingIO{release: controller.readRelease}, controller: controller, timeout: 20 * time.Millisecond,
	}
	started := time.Now()
	_, err := reader.Read(make([]byte, 1))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked read error = %v", err)
	}
	if elapsed := time.Since(started); elapsed < 10*time.Millisecond || elapsed > time.Second {
		t.Fatalf("blocked read deadline elapsed = %s", elapsed)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	if !reader.reader.(*deadlineBlockingIO).closed {
		t.Fatal("deadline reader did not forward Close to the request body")
	}
}

func TestVolumeTransferDeadlineWriterInterruptsBlockedWrite(t *testing.T) {
	controller := newDeadlineControllerStub()
	writer := &volumeTransferDeadlineWriter{
		writer: &deadlineBlockingIO{release: controller.writeRelease}, controller: controller, timeout: 20 * time.Millisecond,
	}
	started := time.Now()
	_, err := writer.Write([]byte("archive"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked write error = %v", err)
	}
	if elapsed := time.Since(started); elapsed < 10*time.Millisecond || elapsed > time.Second {
		t.Fatalf("blocked write deadline elapsed = %s", elapsed)
	}
}

type deadlineControllerStub struct {
	readRelease  chan struct{}
	writeRelease chan struct{}
	readOnce     sync.Once
	writeOnce    sync.Once
}

func newDeadlineControllerStub() *deadlineControllerStub {
	return &deadlineControllerStub{readRelease: make(chan struct{}), writeRelease: make(chan struct{})}
}

func (controller *deadlineControllerStub) SetReadDeadline(deadline time.Time) error {
	controller.readOnce.Do(func() {
		time.AfterFunc(time.Until(deadline), func() { close(controller.readRelease) })
	})
	return nil
}

func (controller *deadlineControllerStub) SetWriteDeadline(deadline time.Time) error {
	controller.writeOnce.Do(func() {
		time.AfterFunc(time.Until(deadline), func() { close(controller.writeRelease) })
	})
	return nil
}

type deadlineBlockingIO struct {
	release <-chan struct{}
	closed  bool
}

func (stream *deadlineBlockingIO) Read([]byte) (int, error) {
	<-stream.release
	return 0, context.DeadlineExceeded
}

func (stream *deadlineBlockingIO) Write([]byte) (int, error) {
	<-stream.release
	return 0, context.DeadlineExceeded
}

func (stream *deadlineBlockingIO) Close() error {
	stream.closed = true
	return nil
}

var _ io.Reader = (*deadlineBlockingIO)(nil)
var _ io.Writer = (*deadlineBlockingIO)(nil)
