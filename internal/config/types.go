package config

import (
	"time"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/LiteyukiStudio/devops/internal/secret"
)

// TelemetryConfig is the validated process telemetry snapshot. Telemetry code
// receives this value and never reads deployment configuration itself.
type TelemetryConfig struct {
	Endpoint           string
	Headers            map[string]string
	ResourceAttributes map[string]string
	LogFormat          string
	LogColor           string
	LogLevel           string
	NoColor            bool
}

// Shared contains values consumed by both API and Worker. It is immutable
// after startup and is explicitly passed to downstream services.
type Shared struct {
	Mode                   string
	PublicBaseURL          string
	DatabaseURL            string
	RedisAddr              string
	SecretCodec            secret.Codec
	VolumeTransferMaxBytes int64
	VolumeTransferJobImage string
	Telemetry              TelemetryConfig
}

type InitialAdminConfig struct {
	Email            string
	Name             string
	Password         string
	Language         string
	FreeQuotaCredits string
}

type APIConfig struct {
	Shared
	Addr                    string
	DatabaseMaxOpenConns    int
	DatabaseMaxIdleConns    int
	DatabaseConnMaxLifetime time.Duration
	DatabaseConnMaxIdleTime time.Duration
	TrustedProxyCIDRs       []string
	InitialAdmin            InitialAdminConfig
	MetricsEnabled          bool
	MetricsAddr             string
	MetricsPath             string
	AllowedOrigins          []string
	EnableHSTS              bool
	AppVersion              string
	BrowserTraceEndpoint    string
	BrowserTraceHeaders     map[string]string
	AIAgent                 aiagent.Config
}

type WorkerConfig struct {
	Shared
	DatabaseMaxOpenConns        int
	DatabaseMaxIdleConns        int
	DatabaseConnMaxLifetime     time.Duration
	DatabaseConnMaxIdleTime     time.Duration
	BuildExecutorImage          string
	BuildEgressMode             string
	BuildCacheEnabled           bool
	BuildCacheTag               string
	BuildJobTimeoutSeconds      int64
	BuildJobTTLSeconds          int64
	BuildPrivateEgressCIDRs     []string
	BuildPrivateEgressPorts     []int
	BuildBlockedEgressCIDRs     []string
	DeployRolloutTimeoutSeconds int64
	CertManagerClusterIssuer    string
}

type TasksConfig struct {
	Redis     redisconfig.Options
	Telemetry TelemetryConfig
}

func (c Shared) RedisOptions() redisconfig.Options {
	return redisconfig.MustParse(c.RedisAddr)
}

func (c Shared) VolumeTransferEnabled() bool {
	return c.VolumeTransferJobImage != ""
}
