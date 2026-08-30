package worker

import sharedconfig "github.com/LiteyukiStudio/devops/internal/config"

type Config = sharedconfig.WorkerConfig

// LoadConfig keeps the Worker package boundary while delegating all
// environment ownership and validation to internal/config.
func LoadConfig() (Config, error) {
	return sharedconfig.LoadWorker()
}
