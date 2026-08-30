package api

import sharedconfig "github.com/LiteyukiStudio/devops/internal/config"

type Config = sharedconfig.APIConfig
type InitialAdminConfig = sharedconfig.InitialAdminConfig

// LoadConfig keeps the API package boundary while delegating all environment
// ownership and primitive validation to internal/config.
func LoadConfig() (Config, error) {
	return sharedconfig.LoadAPI()
}
