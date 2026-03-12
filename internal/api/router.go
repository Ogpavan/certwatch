package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"domainguard/internal/alerts"
	"domainguard/internal/auth"
	"domainguard/internal/domains"
	"domainguard/internal/logs"
	"domainguard/internal/middleware"
	"domainguard/internal/projects"
	"domainguard/internal/settings"
	"domainguard/internal/worker"
	"github.com/gin-gonic/gin"
)

type Config struct {
	JWTSecret string
	TokenTTL  time.Duration
}

func NewRouter(db *sql.DB, scheduler *worker.Scheduler, cfg Config) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	authHandler := &auth.Handler{DB: db, JWTSecret: cfg.JWTSecret, TokenTTL: cfg.TokenTTL}
	projectHandler := &projects.Handler{DB: db}
	domainHandler := &domains.Handler{DB: db}
	alertHandler := &alerts.Handler{DB: db}
	settingsHandler := &settings.Handler{DB: db}
	logsHandler := &logs.Handler{DB: db}

	router.POST("/auth/register", authHandler.Register)
	router.POST("/auth/login", authHandler.Login)

	protected := router.Group("/")
	protected.Use(middleware.AuthMiddleware(cfg.JWTSecret))

	protected.GET("/projects", projectHandler.List)
	protected.POST("/projects", projectHandler.Create)
	protected.DELETE("/projects/:id", projectHandler.Delete)

	protected.GET("/domains", domainHandler.List)
	protected.POST("/domains", domainHandler.Create)
	protected.DELETE("/domains/:id", domainHandler.Delete)
	protected.GET("/domains/:id", domainHandler.Get)
	protected.GET("/domains/:id/history", domainHandler.History)

	protected.GET("/alerts", alertHandler.List)
	protected.POST("/alerts/:id/resolve", alertHandler.Resolve)
	protected.GET("/logs", logsHandler.List)

	protected.GET("/settings/notifications", settingsHandler.GetNotifications)
	protected.PUT("/settings/notifications", settingsHandler.UpdateNotifications)

	protected.POST("/scan-now", func(c *gin.Context) {
		runID := fmt.Sprintf("manual-%d", time.Now().UnixNano())
		if err := scheduler.ScanNow(c.Request.Context(), runID); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(202, gin.H{"status": "scan queued", "run_id": runID})
	})

	protected.GET("/scan-progress/:run_id", func(c *gin.Context) {
		runID := c.Param("run_id")
		events, done := scheduler.GetProgress(runID)
		c.JSON(200, gin.H{"events": events, "done": done})
	})

	attachFrontend(router, "dist")

	return router
}

func attachFrontend(router *gin.Engine, distDir string) {
	if distDir == "" {
		return
	}
	distPath := filepath.Clean(distDir)
	indexPath := filepath.Join(distPath, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return
	}

	router.Static("/assets", filepath.Join(distPath, "assets"))
	router.StaticFile("/favicon.ico", filepath.Join(distPath, "favicon.ico"))
	router.NoRoute(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}
		c.File(indexPath)
	})
}
