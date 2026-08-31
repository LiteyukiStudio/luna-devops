package kubeproxy

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/proxy"
	"k8s.io/klog/v2"
)

type UpgradeProxy struct {
	Telemetry *Telemetry
}

func (upgrade UpgradeProxy) Serve(writer http.ResponseWriter, request *http.Request, upstream Upstream, kubePath string) error {
	upstreamRequest, err := BuildUpstreamRequest(request, upstream, kubePath)
	if err != nil {
		return err
	}
	restoreUpgradeHeaders(upstreamRequest.Header, request.Header)
	baseUpgrade := upstream.UpgradeTransport
	if baseUpgrade == nil {
		baseUpgrade = proxy.NewUpgradeRequestRoundTripper(upstream.Transport, upstream.Transport)
	}
	tracing := TracingUpgradeTransport{Next: baseUpgrade, Telemetry: upgrade.Telemetry}
	responder := &safeUpgradeResponder{}
	handler := proxy.NewUpgradeAwareHandler(upstreamRequest.URL, upstream.Transport, false, true, responder)
	handler.AppendLocationPath = false
	handler.UseRequestLocation = false
	handler.UseLocationHost = true
	handler.UpgradeTransport = tracing
	handler.WrapTransport = false
	handler.RejectForwardingRedirects = true
	upstreamRequest = upstreamRequest.WithContext(klog.NewContext(upstreamRequest.Context(), logr.Discard()))
	handler.ServeHTTP(writer, upstreamRequest)
	return responder.Err()
}

func restoreUpgradeHeaders(target, source http.Header) {
	target.Set("Connection", "Upgrade")
	target.Set("Upgrade", source.Get("Upgrade"))
	for name, values := range source {
		lower := strings.ToLower(name)
		if !strings.HasPrefix(lower, "sec-websocket-") && lower != "x-stream-protocol-version" {
			continue
		}
		target.Del(name)
		for _, value := range values {
			target.Add(name, value)
		}
	}
}

type safeUpgradeResponder struct {
	mu  sync.Mutex
	err error
}

func (responder *safeUpgradeResponder) Error(writer http.ResponseWriter, _ *http.Request, err error) {
	responder.mu.Lock()
	responder.err = Unavailable(CodeUnavailable, fmt.Errorf("upgrade proxy failed: %w", err))
	responder.mu.Unlock()
	WriteStatus(writer, responder.err)
}

func (responder *safeUpgradeResponder) Err() error {
	responder.mu.Lock()
	defer responder.mu.Unlock()
	return responder.err
}
