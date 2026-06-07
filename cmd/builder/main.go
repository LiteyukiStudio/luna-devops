package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/LiteyukiStudio/devops/internal/builder"
	"github.com/LiteyukiStudio/devops/internal/config"
)

func main() {
	cfg := config.LoadBuilder()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	agent := builder.New(builder.Options{
		RedisAddr:         cfg.RedisAddr,
		Name:              cfg.BuilderAgentName,
		Executor:          cfg.BuilderExecutor,
		ExecutorImage:     cfg.BuilderExecutorImage,
		Labels:            cfg.BuilderLabels,
		MaxConcurrency:    cfg.BuilderMaxConcurrency,
		Scopes:            cfg.BuilderScopes,
		PollInterval:      time.Duration(cfg.BuilderPollIntervalSeconds) * time.Second,
		WorkspaceRoot:     cfg.BuilderWorkspaceRoot,
		WorkspaceHostRoot: cfg.BuilderWorkspaceHostRoot,
		NPMRegistry:       cfg.BuilderNPMRegistry,
	})
	if err := agent.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("run builder: %v", err)
	}
}
