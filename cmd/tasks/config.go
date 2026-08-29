package main

import (
	"fmt"
	"strings"

	sharedconfig "github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/redisconfig"
)

type tasksConfig struct {
	Redis redisconfig.Options
}

func loadTasksConfig() (tasksConfig, error) {
	address := strings.TrimSpace(sharedconfig.String("REDIS_ADDR", "redis://localhost:6379/0"))
	options, err := redisconfig.Parse(address)
	if err != nil {
		return tasksConfig{}, fmt.Errorf("invalid REDIS_ADDR: %w", err)
	}
	return tasksConfig{Redis: options}, nil
}
