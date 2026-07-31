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
	ctx := context.Background()
	runtime, err := telemetry.Setup(ctx, telemetry.ServiceConfig{ServiceName: "luna-gateway-traffic-probe"})
	if err != nil {
		_, _ = os.Stderr.WriteString("initialize telemetry failed\n")
		os.Exit(1)
	}
	defer func() { _ = runtime.Shutdown(context.Background()) }()
	logger := telemetry.Logger()

	cfg, err := gatewayprobe.ConfigFromEnv()
	if err != nil {
		logger.Error("invalid gateway traffic probe config", "event.name", "gateway_probe.config.invalid", "error.type", telemetry.ErrorType(err))
		os.Exit(1)
	}
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		logger.Error("load in-cluster Kubernetes config", "event.name", "gateway_probe.kubernetes.config_failed", "error.type", telemetry.ErrorType(err))
		os.Exit(1)
	}
	discoverer, err := gatewayprobe.NewGatewayAPIRouteDiscoverer(kubeConfig)
	if err != nil {
		logger.Error("create gateway route discoverer", "event.name", "gateway_probe.discoverer.failed", "error.type", telemetry.ErrorType(err))
		os.Exit(1)
	}
	reporter := gatewayprobe.NewAPIReporter(cfg.APIBaseURL, cfg.ReportToken, cfg.HTTPTimeout)
	collector := gatewayprobe.NewCollector(cfg, discoverer, reporter, logger)

	signalCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", collector.Healthz)
	mux.HandleFunc("/metrics", collector.Metrics)
	server := &http.Server{Addr: cfg.ProbeAddr, Handler: otelhttp.NewHandler(mux, "gateway_probe.status"), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("gateway traffic probe status server started", "addr", cfg.ProbeAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("status server failed", "event.name", "gateway_probe.status.failed", "error.type", telemetry.ErrorType(err))
			stop()
		}
	}()
	go func() {
		if err := collector.Run(signalCtx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("collector stopped", "event.name", "gateway_probe.collector.failed", "error.type", telemetry.ErrorType(err))
			stop()
		}
	}()

	<-signalCtx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	logger.Info("gateway traffic probe stopped")
}
