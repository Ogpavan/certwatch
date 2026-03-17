package config

import (
	"os"
	"strconv"
	"time"
)

const (
	defaultConnString = "Data Source=(localdb)\\MSSQLLocalDB;Integrated Security=True;Persist Security Info=False;Pooling=False;MultipleActiveResultSets=False;Encrypt=True;TrustServerCertificate=False;Application Name=\"DomainGuard\";Command Timeout=0"
	defaultDBName     = "DomainGuardDB"
	defaultAPIPort    = "8080"
	defaultWorkerPort = "8090"
)

type AppConfig struct {
	ConnString      string
	DBName          string
	JWTSecret       string
	TokenTTL        time.Duration
	Port            string
	Workers         int
	ScannerInterval time.Duration
	SMTPHost        string
	SMTPPort        int
	SMTPUser        string
	SMTPPass        string
	SMTPFrom        string

	WorkerPort        string
	WorkerBind        string
	WorkerAPIURL      string
	WorkerAPIToken    string
	WorkerTimeoutSecs int
	JobQueueSize      int
	DispatchQueueSize int
}

func LoadAppConfig() AppConfig {
	cfg := AppConfig{
		ConnString:        defaultConnString,
		DBName:            defaultDBName,
		JWTSecret:         envOrDefault("JWT_SECRET", "change-me"),
		Port:              envOrDefault("PORT", defaultAPIPort),
		Workers:           envInt("WORKER_COUNT", 5),
		ScannerInterval:   time.Duration(envInt("SCANNER_INTERVAL_HOURS", 24)) * time.Hour,
		SMTPHost:          envOrDefault("SMTP_HOST", "smtp.gmail.com"),
		SMTPPort:          envInt("SMTP_PORT", 587),
		SMTPUser:          envOrDefault("SMTP_USER", ""),
		SMTPPass:          envOrDefault("SMTP_PASS", ""),
		SMTPFrom:          envOrDefault("SMTP_FROM", ""),
		WorkerPort:        envOrDefault("WORKER_PORT", defaultWorkerPort),
		WorkerBind:        envOrDefault("WORKER_BIND", "127.0.0.1"),
		WorkerAPIURL:      envOrDefault("WORKER_API_URL", "http://127.0.0.1:"+defaultWorkerPort),
		WorkerAPIToken:    envOrDefault("WORKER_API_TOKEN", ""),
		WorkerTimeoutSecs: envInt("WORKER_API_TIMEOUT_SECONDS", 10),
		JobQueueSize:      envInt("WORKER_JOB_QUEUE_SIZE", 10000),
		DispatchQueueSize: envInt("WORKER_DISPATCH_QUEUE_SIZE", 64),
	}

	if value := os.Getenv("DB_CONN_STRING"); value != "" {
		cfg.ConnString = value
	}
	if value := os.Getenv("DB_NAME"); value != "" {
		cfg.DBName = value
	}
	if value := os.Getenv("TOKEN_TTL_HOURS"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			cfg.TokenTTL = time.Duration(parsed) * time.Hour
		}
	}
	if cfg.TokenTTL == 0 {
		cfg.TokenTTL = 24 * time.Hour
	}
	if cfg.WorkerTimeoutSecs <= 0 {
		cfg.WorkerTimeoutSecs = 10
	}
	if cfg.JobQueueSize <= 0 {
		cfg.JobQueueSize = 10000
	}
	if cfg.DispatchQueueSize <= 0 {
		cfg.DispatchQueueSize = 64
	}

	return cfg
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
