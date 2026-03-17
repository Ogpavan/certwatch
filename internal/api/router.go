package api

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"domainguard/internal/alerts"
	"domainguard/internal/domains"
	"domainguard/internal/logs"
	"domainguard/internal/middleware"
	"domainguard/internal/projects"
	"domainguard/internal/settings"
	"domainguard/internal/worker"
	"github.com/gin-gonic/gin"
)

type Config struct {
	JWTSecret      string
	TokenTTL       time.Duration
	WorkerAPIURL   string
	WorkerAPIToken string
	WorkerTimeout  time.Duration
	DefaultUserID  int
}

func NewRouter(db *sql.DB, scheduler *worker.Scheduler, cfg Config) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	projectHandler := &projects.Handler{DB: db}
	domainHandler := &domains.Handler{DB: db}
	alertHandler := &alerts.Handler{DB: db}
	settingsHandler := &settings.Handler{DB: db}
	logsHandler := &logs.Handler{DB: db}

	router.Use(middleware.DefaultUserMiddleware(cfg.DefaultUserID))

	router.GET("/projects", projectHandler.List)
	router.POST("/projects", projectHandler.Create)
	router.DELETE("/projects/:id", projectHandler.Delete)

	router.GET("/domains", domainHandler.List)
	router.POST("/domains", domainHandler.Create)
	router.DELETE("/domains/:id", domainHandler.Delete)
	router.GET("/domains/:id", domainHandler.Get)
	router.GET("/domains/:id/history", domainHandler.History)

	router.GET("/alerts", alertHandler.List)
	router.POST("/alerts/:id/resolve", alertHandler.Resolve)
	router.GET("/logs", logsHandler.List)

	router.GET("/settings/notifications", settingsHandler.GetNotifications)
	router.PUT("/settings/notifications", settingsHandler.UpdateNotifications)

	router.POST("/scan-now", func(c *gin.Context) {
		if scheduler != nil {
			runID := fmt.Sprintf("manual-%d", time.Now().UnixNano())
			if err := scheduler.ScanNow(c.Request.Context(), runID); err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			c.JSON(202, gin.H{"status": "scan queued", "run_id": runID})
			return
		}
		proxyScanNow(c, cfg)
	})

	router.GET("/scan-progress/:run_id", func(c *gin.Context) {
		if scheduler != nil {
			runID := c.Param("run_id")
			events, done := scheduler.GetProgress(runID)
			c.JSON(200, gin.H{"events": events, "done": done})
			return
		}
		proxyScanProgress(c, cfg)
	})

	attachFrontend(router, "dist")

	return router
}

func proxyScanNow(c *gin.Context, cfg Config) {
	if strings.TrimSpace(cfg.WorkerAPIURL) == "" {
		c.JSON(503, gin.H{"error": "worker api not configured"})
		return
	}
	client := &http.Client{Timeout: sanitizeTimeout(cfg.WorkerTimeout)}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, strings.TrimRight(cfg.WorkerAPIURL, "/")+"/scan-now", nil)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to build request"})
		return
	}
	if strings.TrimSpace(cfg.WorkerAPIToken) != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.WorkerAPIToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(502, gin.H{"error": "worker api unavailable"})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, "application/json", body)
}

func proxyScanProgress(c *gin.Context, cfg Config) {
	if strings.TrimSpace(cfg.WorkerAPIURL) == "" {
		c.JSON(503, gin.H{"error": "worker api not configured"})
		return
	}
	runID := c.Param("run_id")
	client := &http.Client{Timeout: sanitizeTimeout(cfg.WorkerTimeout)}
	url := strings.TrimRight(cfg.WorkerAPIURL, "/") + "/scan-progress/" + runID
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, url, nil)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to build request"})
		return
	}
	if strings.TrimSpace(cfg.WorkerAPIToken) != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.WorkerAPIToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(502, gin.H{"error": "worker api unavailable"})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, "application/json", body)
}

func sanitizeTimeout(value time.Duration) time.Duration {
	if value <= 0 {
		return 10 * time.Second
	}
	return value
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
