package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	appconfig "domainguard/internal/config"
	"domainguard/internal/database"
	"domainguard/internal/notifications"
	"domainguard/internal/worker"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	emailer := &notifications.EmailSender{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		Username: cfg.SMTPUser,
		Password: cfg.SMTPPass,
		From:     cfg.SMTPFrom,
	}
	scheduler := &worker.Scheduler{
		DB:                db,
		Workers:           cfg.Workers,
		Interval:          cfg.ScannerInterval,
		Emailer:           emailer,
		JobQueueSize:      cfg.JobQueueSize,
		DispatchQueueSize: cfg.DispatchQueueSize,
	}
	scheduler.Start(ctx)

	gin.SetMode(gin.ReleaseMode)
	router := worker.NewWorkerRouter(scheduler, worker.HTTPConfig{Token: cfg.WorkerAPIToken})
	server := &http.Server{Addr: cfg.WorkerBind + ":" + cfg.WorkerPort, Handler: router}

	go func() {
		log.Printf("worker api running on http://%s:%s", cfg.WorkerBind, cfg.WorkerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("worker server error: %v", err)
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	<-shutdown

	log.Println("shutting down worker...")
	cancel()
	ctxTimeout, cancelTimeout := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelTimeout()
	if err := server.Shutdown(ctxTimeout); err != nil {
		log.Printf("worker shutdown error: %v", err)
	}
}
