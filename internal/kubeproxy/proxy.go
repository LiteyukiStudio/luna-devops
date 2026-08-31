package kubeproxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type ResponseTransformer interface {
	Transform(context.Context, *http.Response) error
}

type ResponseTransformFunc func(context.Context, *http.Response) error

func (function ResponseTransformFunc) Transform(ctx context.Context, response *http.Response) error {
	return function(ctx, response)
}

type HTTPProxy struct {
	FlushInterval time.Duration
	Telemetry     *Telemetry
}

func (proxy HTTPProxy) Serve(writer http.ResponseWriter, request *http.Request, upstream Upstream, kubePath string, info RequestInfo, transformer ResponseTransformer) error {
	upstreamRequest, err := BuildUpstreamRequest(request, upstream, kubePath)
	if err != nil {
		return err
	}
	telemetry := proxy.Telemetry
	if telemetry == nil {
		telemetry = NewTelemetry(nil)
	}
	proxyCtx, proxySpan := telemetry.StartInternal(upstreamRequest.Context(), "kubernetes.proxy", trace.SpanKindClient)
	upstreamRequest = upstreamRequest.WithContext(proxyCtx)
	telemetry.InjectUpstream(proxyCtx, upstreamRequest.Header)
	response, err := upstream.Transport.RoundTrip(upstreamRequest)
	if err != nil {
		proxySpan.SetStatus(codes.Error, CodeUnavailable)
		proxySpan.End()
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(request.Context().Err(), context.DeadlineExceeded) {
			return GatewayTimeout(err)
		}
		return Unavailable(CodeUnavailable, err)
	}
	if response == nil || response.Body == nil {
		proxySpan.SetStatus(codes.Error, CodeUnavailable)
		proxySpan.End()
		return Unavailable(CodeUnavailable, errors.New("upstream returned an empty response"))
	}
	defer func() { _ = response.Body.Close() }()
	defer proxySpan.End()
	if response.StatusCode >= http.StatusInternalServerError {
		proxySpan.SetStatus(codes.Error, CodeUnavailable)
	} else {
		proxySpan.SetStatus(codes.Ok, "")
	}
	if transformer != nil {
		if err := transformer.Transform(request.Context(), response); err != nil {
			return err
		}
	}
	copyResponseHeaders(writer.Header(), response.Header)
	if info.IsResourceRequest {
		writer.Header().Set("Cache-Control", "no-store")
	}
	writer.WriteHeader(response.StatusCode)
	if request.Method == http.MethodHead {
		return nil
	}
	flushInterval := proxy.FlushInterval
	if flushInterval <= 0 {
		flushInterval = 100 * time.Millisecond
	}
	if info.Transport == TransportWatch || info.Transport == TransportLogs {
		_, err = io.CopyBuffer(periodicFlushWriter(writer, flushInterval), &streamActivityReader{ctx: request.Context(), reader: response.Body}, make([]byte, 32<<10))
	} else {
		_, err = io.CopyBuffer(writer, response.Body, make([]byte, 32<<10))
	}
	if err != nil && request.Context().Err() == nil {
		return Unavailable(CodeUnavailable, err)
	}
	return request.Context().Err()
}

type streamActivityReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *streamActivityReader) Read(data []byte) (int, error) {
	read, err := reader.reader.Read(data)
	if read > 0 {
		touchStream(reader.ctx)
	}
	return read, err
}

func copyResponseHeaders(target, source http.Header) {
	for name, values := range source {
		if hopByHopResponseHeader(name, source) {
			continue
		}
		for _, value := range values {
			target.Add(name, value)
		}
	}
}

func hopByHopResponseHeader(name string, header http.Header) bool {
	lower := strings.ToLower(name)
	switch lower {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade", "set-cookie":
		return true
	}
	for _, value := range header.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), name) {
				return true
			}
		}
	}
	return false
}

type flushResponseWriter struct {
	writer    io.Writer
	flusher   http.Flusher
	interval  time.Duration
	lastFlush time.Time
}

func periodicFlushWriter(writer http.ResponseWriter, interval time.Duration) io.Writer {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		return writer
	}
	return &flushResponseWriter{writer: writer, flusher: flusher, interval: interval, lastFlush: time.Now()}
}

func (writer *flushResponseWriter) Write(data []byte) (int, error) {
	written, err := writer.writer.Write(data)
	if time.Since(writer.lastFlush) >= writer.interval {
		writer.flusher.Flush()
		writer.lastFlush = time.Now()
	}
	return written, err
}
