package kubeproxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type upgradeSpanContextKey struct{}

type upgradeSpanState struct {
	span trace.Span
	once sync.Once
}

func (state *upgradeSpanState) end(err error, status int) {
	if state == nil {
		return
	}
	state.once.Do(func() {
		if err != nil || status >= http.StatusBadRequest {
			state.span.SetStatus(codes.Error, CodeUnavailable)
		} else {
			state.span.SetStatus(codes.Ok, "")
		}
		state.span.End()
	})
}

type TracingUpgradeTransport struct {
	Next      UpgradeRequestRoundTripper
	Telemetry *Telemetry
}

func (transport TracingUpgradeTransport) WrapRequest(request *http.Request) (*http.Request, error) {
	if transport.Next == nil || request == nil {
		return nil, Unavailable(CodeUnavailable, fmt.Errorf("upgrade transport is unavailable"))
	}
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	SanitizeUpstreamHeaders(clone.Header, true)
	wrapped, err := transport.Next.WrapRequest(clone)
	if err != nil {
		return nil, Unavailable(CodeUnavailable, err)
	}
	telemetry := transport.Telemetry
	if telemetry == nil {
		telemetry = NewTelemetry(nil)
	}
	ctx, span := telemetry.StartInternal(wrapped.Context(), "kubernetes.proxy", trace.SpanKindClient)
	state := &upgradeSpanState{span: span}
	telemetry.InjectUpstream(ctx, wrapped.Header)
	return wrapped.WithContext(context.WithValue(ctx, upgradeSpanContextKey{}, state)), nil
}

func (transport TracingUpgradeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if transport.Next == nil {
		return nil, Unavailable(CodeUnavailable, fmt.Errorf("upgrade transport is unavailable"))
	}
	state, _ := request.Context().Value(upgradeSpanContextKey{}).(*upgradeSpanState)
	if state == nil {
		wrapped, err := transport.WrapRequest(request)
		if err != nil {
			return nil, err
		}
		request = wrapped
		state, _ = request.Context().Value(upgradeSpanContextKey{}).(*upgradeSpanState)
	}
	response, err := transport.Next.RoundTrip(request)
	if err != nil {
		state.end(err, 0)
		return nil, err
	}
	if response == nil || response.Body == nil {
		state.end(fmt.Errorf("empty upgrade response"), 0)
		return response, nil
	}
	if stream, ok := response.Body.(io.ReadWriteCloser); ok {
		response.Body = &spanReadWriteCloser{ReadWriteCloser: stream, state: state, status: response.StatusCode, ctx: request.Context()}
	} else {
		response.Body = &spanReadCloser{ReadCloser: response.Body, state: state, status: response.StatusCode, ctx: request.Context()}
	}
	return response, nil
}

type spanReadCloser struct {
	io.ReadCloser
	state  *upgradeSpanState
	status int
	ctx    context.Context
}

func (body *spanReadCloser) Read(data []byte) (int, error) {
	read, err := body.ReadCloser.Read(data)
	if read > 0 {
		touchStream(body.ctx)
	}
	return read, err
}

func (body *spanReadCloser) Close() error {
	err := body.ReadCloser.Close()
	body.state.end(err, body.status)
	return err
}

type spanReadWriteCloser struct {
	io.ReadWriteCloser
	state  *upgradeSpanState
	status int
	ctx    context.Context
}

func (body *spanReadWriteCloser) Read(data []byte) (int, error) {
	read, err := body.ReadWriteCloser.Read(data)
	if read > 0 {
		touchStream(body.ctx)
	}
	return read, err
}

func (body *spanReadWriteCloser) Write(data []byte) (int, error) {
	written, err := body.ReadWriteCloser.Write(data)
	if written > 0 {
		touchStream(body.ctx)
	}
	return written, err
}

func (body *spanReadWriteCloser) Close() error {
	err := body.ReadWriteCloser.Close()
	body.state.end(err, body.status)
	return err
}
