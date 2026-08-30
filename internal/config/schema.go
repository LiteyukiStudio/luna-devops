package config

import "time"

type databasePoolEnvironment struct {
	MaxOpenConns    int           `env:"MAX_OPEN_CONNS" envDefault:"20"`
	MaxIdleConns    int           `env:"MAX_IDLE_CONNS" envDefault:"5"`
	ConnMaxLifetime time.Duration `env:"CONN_MAX_LIFETIME" envDefault:"30m"`
	ConnMaxIdleTime time.Duration `env:"CONN_MAX_IDLE_TIME" envDefault:"5m"`
}

type telemetryEnvironment struct {
	Endpoint           string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	Headers            string `env:"OTEL_EXPORTER_OTLP_HEADERS"`
	ResourceAttributes string `env:"OTEL_RESOURCE_ATTRIBUTES"`
	LogFormat          string `env:"LOG_FORMAT" envDefault:"auto"`
	LogColor           string `env:"LOG_COLOR" envDefault:"auto"`
	LogLevel           string `env:"LOG_LEVEL" envDefault:"info"`
}

type sharedEnvironment struct {
	Mode                   string `env:"APP_ENV" envDefault:"production"`
	PublicBaseURL          string `env:"PUBLIC_BASE_URL"`
	DatabaseURL            string `env:"DATABASE_URL" envDefault:"postgres://devops:devops@localhost:5432/devops?sslmode=disable"`
	RedisAddr              string `env:"REDIS_ADDR" envDefault:"redis://localhost:6379/0"`
	SecretEncryptionKey    string `env:"SECRET_ENCRYPTION_KEY"`
	VolumeTransferMaxBytes string `env:"VOLUME_TRANSFER_MAX_BYTES" envDefault:"100Gi"`
	VolumeTransferJobImage string `env:"VOLUME_TRANSFER_JOB_IMAGE"`
	Telemetry              telemetryEnvironment
}

type initialAdminEnvironment struct {
	Email            string `env:"INITIAL_ADMIN_EMAIL"`
	Name             string `env:"INITIAL_ADMIN_NAME"`
	Password         string `env:"INITIAL_ADMIN_PASSWORD"`
	Language         string `env:"INITIAL_ADMIN_LANGUAGE"`
	FreeQuotaCredits string `env:"LOCAL_ADMIN_FREE_QUOTA_CREDITS" envDefault:"1000"`
}

type apiEnvironment struct {
	Addr                 string                  `env:"API_ADDR" envDefault:":8080"`
	Database             databasePoolEnvironment `envPrefix:"API_DB_"`
	TrustedProxyCIDRs    string                  `env:"TRUSTED_PROXY_CIDRS"`
	InitialAdmin         initialAdminEnvironment
	MetricsEnabled       bool          `env:"METRICS_ENABLED" envDefault:"false"`
	MetricsAddr          string        `env:"METRICS_ADDR"`
	MetricsPath          string        `env:"METRICS_PATH" envDefault:"/metrics"`
	EnableHSTS           *bool         `env:"APP_ENABLE_HSTS"`
	CORSOrigins          string        `env:"APP_CORS_ORIGINS"`
	AppVersion           string        `env:"APP_VERSION" envDefault:"dev"`
	TraceEndpoint        string        `env:"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"`
	TraceHeaders         string        `env:"OTEL_EXPORTER_OTLP_TRACES_HEADERS"`
	AIAssistantAvailable bool          `env:"AI_ASSISTANT_AVAILABLE" envDefault:"false"`
	AIAgentTimeout       time.Duration `env:"AI_AGENT_TIMEOUT" envDefault:"10s"`
	AIAgentBaseURL       string        `env:"AI_AGENT_BASE_URL"`
	AIInternalSecret     string        `env:"AI_INTERNAL_SECRET"`
}

type workerEnvironment struct {
	Database                 databasePoolEnvironment `envPrefix:"WORKER_DB_"`
	BuildExecutorImage       string                  `env:"BUILD_EXECUTOR_IMAGE" envDefault:"moby/buildkit:v0.24.0-rootless"`
	BuildEgressMode          string                  `env:"BUILD_EGRESS_MODE" envDefault:"restricted"`
	BuildCacheEnabled        bool                    `env:"BUILD_CACHE_ENABLED" envDefault:"false"`
	BuildCacheTag            string                  `env:"BUILD_CACHE_TAG" envDefault:"buildcache"`
	BuildJobTimeoutSeconds   int                     `env:"BUILD_JOB_TIMEOUT_SECONDS" envDefault:"1800"`
	BuildJobTTLSeconds       int                     `env:"BUILD_JOB_TTL_SECONDS" envDefault:"3600"`
	BuildPrivateEgressCIDRs  string                  `env:"BUILD_PRIVATE_EGRESS_CIDRS"`
	BuildPrivateEgressPorts  string                  `env:"BUILD_PRIVATE_EGRESS_PORTS" envDefault:"443"`
	BuildBlockedEgressCIDRs  string                  `env:"BUILD_BLOCKED_EGRESS_CIDRS"`
	DeployRolloutTimeout     int                     `env:"DEPLOY_ROLLOUT_TIMEOUT_SECONDS" envDefault:"600"`
	CertManagerClusterIssuer string                  `env:"CERT_MANAGER_CLUSTER_ISSUER" envDefault:"letsencrypt-http01"`
}

type tasksEnvironment struct {
	RedisAddr string `env:"REDIS_ADDR" envDefault:"redis://localhost:6379/0"`
}
