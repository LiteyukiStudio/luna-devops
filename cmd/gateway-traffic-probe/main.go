package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LiteyukiStudio/devops/internal/gatewayprobe"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"k8s.io/client-go/rest"
)

func main() {
	os.Exit(runMain())
}

func runMain() int {
	ctx := context.Background()
	runtime, err := telemetry.Setup(ctx, telemetry.ServiceConfig{ServiceName: "luna-gateway-traffic-probe"})
	if err != nil {
		telemetry.LogError(ctx, "Gateway traffic probe startup failed", "gateway_probe.startup.failed",
			"gateway_probe.startup", "telemetry.initialization.failed",
			telemetry.WrapError("telemetry.initialization.failed", "verify the OTEL exporter configuration", "initialize telemetry", err))
		return 1
	}
	defer func() { _ = runtime.Shutdown(context.Background()) }()
	logger := telemetry.Logger()

	cfg, err := gatewayprobe.ConfigFromEnv()
	if err != nil {
		telemetry.LogError(ctx, "Gateway traffic probe startup failed", "gateway_probe.startup.failed",
			"gateway_probe.startup", "config.invalid",
			telemetry.WrapError("config.invalid", "verify the gateway traffic probe environment", "load gateway traffic probe configuration", err))
		return 1
	}
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		telemetry.LogError(ctx, "Gateway traffic probe startup failed", "gateway_probe.startup.failed",
			"gateway_probe.startup", "kubernetes.request.failed",
			telemetry.WrapError("kubernetes.request.failed", "run the probe in Kubernetes with a service account", "load in-cluster Kubernetes configuration", err))
		return 1
	}
	discoverer, err := gatewayprobe.NewGatewayAPIRouteDiscoverer(kubeConfig)
	if err != nil {
		telemetry.LogError(ctx, "Gateway traffic probe startup failed", "gateway_probe.startup.failed",
			"gateway_probe.startup", "kubernetes.request.failed",
			telemetry.WrapError("kubernetes.request.failed", "verify Kubernetes API access and Gateway API resources", "create gateway route discoverer", err))
		return 1
	}
	reporter := gatewayprobe.NewAPIReporter(cfg.APIBaseURL, cfg.ReportToken, cfg.HTTPTimeout)
	collector := gatewayprobe.NewCollector(cfg, discoverer, reporter, logger)

	signalCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", collector.Healthz)
	mux.HandleFunc("/metrics", collector.Metrics)
	server := &http.Server{Addr: cfg.ProbeAddr, Handler: otelhttp.NewHandler(mux, "gateway_probe.status"), ReadHeaderTimeout: 5 * time.Second}
	failures := make(chan error, 1)
	go func() {
		logger.Info("gateway traffic probe status server started", "event.name", "gateway_probe.status.started", "server.address", cfg.ProbeAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case failures <- telemetry.WrapError("server.listen.failed", "verify GATEWAY_TRAFFIC_PROBE_ADDR is available", "listen on gateway probe status address", err):
			default:
			}
			stop()
		}
	}()
	go func() {
		if err := collector.Run(signalCtx); err != nil && !errors.Is(err, context.Canceled) {
			select {
			case failures <- telemetry.WrapError("kubernetes.request.failed", "verify Kubernetes API and Luna API connectivity", "run gateway traffic collector", err):
			default:
			}
			stop()
		}
	}()

	<-signalCtx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	select {
	case err := <-failures:
		telemetry.LogError(ctx, "Gateway traffic probe failed", "gateway_probe.failed",
			"gateway_probe.run", "gateway_probe.failed", err)
		return 1
	default:
		logger.Info("gateway traffic probe stopped", "event.name", "gateway_probe.stopped", "outcome", "succeeded")
		return 0
	}
}
