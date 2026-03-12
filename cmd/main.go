package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"domainguard/internal/api"
	appconfig "domainguard/internal/config"
	"domainguard/internal/database"
	"domainguard/internal/notifications"
	"domainguard/internal/worker"
	"github.com/gin-gonic/gin"
)

const (
	defaultConnString = "Data Source=(localdb)\\MSSQLLocalDB;Integrated Security=True;Persist Security Info=False;Pooling=False;MultipleActiveResultSets=False;Encrypt=True;TrustServerCertificate=False;Application Name=\"DomainGuard\";Command Timeout=0"
	defaultDBName     = "DomainGuardDB"
	defaultPort       = "8080"
)

func main() {
	appconfig.LoadEnv(".env")
	cfg := loadConfig()

	if err := database.EnsureDatabase(cfg.ConnString, cfg.DBName); err != nil {
		log.Fatalf("failed to ensure database: %v", err)
	}

	db, err := database.Connect(cfg.ConnString, cfg.DBName)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}
	defer db.Close()

	if err := database.EnsureTables(db); err != nil {
		log.Fatalf("failed to ensure tables: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	emailer := &notifications.EmailSender{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		Username: cfg.SMTPUser,
		Password: cfg.SMTPPass,
		From:     cfg.SMTPFrom,
	}
	scheduler := &worker.Scheduler{DB: db, Workers: cfg.Workers, Interval: cfg.ScannerInterval, Emailer: emailer}
	scheduler.Start(ctx)

	gin.SetMode(gin.ReleaseMode)
	router := api.NewRouter(db, scheduler, api.Config{JWTSecret: cfg.JWTSecret, TokenTTL: cfg.TokenTTL})

	server := &http.Server{Addr: ":" + cfg.Port, Handler: router}

	go func() {
		log.Printf("server running on http://localhost:%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	<-shutdown

	log.Println("shutting down...")
	cancel()

	ctxTimeout, cancelTimeout := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelTimeout()
	if err := server.Shutdown(ctxTimeout); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}

type appConfig struct {
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
}

func loadConfig() appConfig {
	cfg := appConfig{
		ConnString:      defaultConnString,
		DBName:          defaultDBName,
		JWTSecret:       envOrDefault("JWT_SECRET", "change-me"),
		Port:            envOrDefault("PORT", defaultPort),
		Workers:         envInt("WORKER_COUNT", 5),
		ScannerInterval: time.Duration(envInt("SCANNER_INTERVAL_HOURS", 24)) * time.Hour,
		SMTPHost:        envOrDefault("SMTP_HOST", "smtp.gmail.com"),
		SMTPPort:        envInt("SMTP_PORT", 587),
		SMTPUser:        envOrDefault("SMTP_USER", ""),
		SMTPPass:        envOrDefault("SMTP_PASS", ""),
		SMTPFrom:        envOrDefault("SMTP_FROM", ""),
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
