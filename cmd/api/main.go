package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"domainguard/internal/api"
	appconfig "domainguard/internal/config"
	"domainguard/internal/database"
	"github.com/gin-gonic/gin"
)

func main() {
	if loc, err := time.LoadLocation("Asia/Kolkata"); err == nil {
		time.Local = loc
	}
	appconfig.LoadEnv(".env")
	cfg := appconfig.LoadAppConfig()

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

	defaultUserID, err := database.EnsureDefaultUser(db)
	if err != nil {
		log.Fatalf("failed to ensure default user: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)
	router := api.NewRouter(db, nil, api.Config{
		JWTSecret:      cfg.JWTSecret,
		TokenTTL:       cfg.TokenTTL,
		WorkerAPIURL:   cfg.WorkerAPIURL,
		WorkerAPIToken: cfg.WorkerAPIToken,
		WorkerTimeout:  time.Duration(cfg.WorkerTimeoutSecs) * time.Second,
		DefaultUserID:  defaultUserID,
	})

	server := &http.Server{Addr: ":" + cfg.Port, Handler: router}

	go func() {
		log.Printf("api running on http://localhost:%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	<-shutdown

	log.Println("shutting down api...")
	ctxTimeout, cancelTimeout := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelTimeout()
	if err := server.Shutdown(ctxTimeout); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}
