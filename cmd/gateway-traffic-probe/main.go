package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LiteyukiStudio/devops/internal/gatewayprobe"
	"k8s.io/client-go/rest"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := gatewayprobe.ConfigFromEnv()
	if err != nil {
		logger.Error("invalid gateway traffic probe config", "error", err)
		os.Exit(1)
	}
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		logger.Error("load in-cluster Kubernetes config", "error", err)
		os.Exit(1)
	}
	discoverer, err := gatewayprobe.NewGatewayAPIRouteDiscoverer(kubeConfig)
	if err != nil {
		logger.Error("create gateway route discoverer", "error", err)
		os.Exit(1)
	}
	reporter := gatewayprobe.NewAPIReporter(cfg.APIBaseURL, cfg.ReportToken, cfg.HTTPTimeout)
	collector := gatewayprobe.NewCollector(cfg, discoverer, reporter, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", collector.Healthz)
	mux.HandleFunc("/metrics", collector.Metrics)
	server := &http.Server{Addr: cfg.ProbeAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("gateway traffic probe status server started", "addr", cfg.ProbeAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("status server failed", "error", err)
			stop()
		}
	}()
	go func() {
		if err := collector.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("collector stopped", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	logger.Info("gateway traffic probe stopped")
}
