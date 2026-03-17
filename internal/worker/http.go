package worker

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type HTTPConfig struct {
	Token string
}

func NewWorkerRouter(scheduler *Scheduler, cfg HTTPConfig) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	if strings.TrimSpace(cfg.Token) != "" {
		router.Use(func(c *gin.Context) {
			auth := c.GetHeader("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")) != cfg.Token {
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
			c.Next()
		})
	}

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	router.POST("/scan-now", func(c *gin.Context) {
		runID := fmt.Sprintf("manual-%d", time.Now().UnixNano())
		if err := scheduler.ScanNow(c.Request.Context(), runID); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(202, gin.H{"status": "scan queued", "run_id": runID})
	})

	router.GET("/scan-progress/:run_id", func(c *gin.Context) {
		runID := c.Param("run_id")
		events, done := scheduler.GetProgress(runID)
		c.JSON(200, gin.H{"events": events, "done": done})
	})

	return router
}
