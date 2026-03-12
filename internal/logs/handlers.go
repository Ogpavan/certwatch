package logs

import (
  "database/sql"
  "net/http"
  "strconv"

  "domainguard/internal/middleware"
  "github.com/gin-gonic/gin"
)

type Handler struct {
  DB *sql.DB
}

func (h *Handler) List(c *gin.Context) {
  userID := middleware.GetUserID(c)
  limit := 200
  if raw := c.Query("limit"); raw != "" {
    if parsed, err := strconv.Atoi(raw); err == nil {
      if parsed > 0 && parsed <= 500 {
        limit = parsed
      }
    }
  }

  rows, err := h.DB.Query(
    `SELECT TOP (@p1) l.id, l.level, l.message, l.created_at, d.domain
     FROM logs l
     LEFT JOIN domains d ON l.domain_id=d.id
     WHERE l.user_id=@p2
     ORDER BY l.created_at DESC`,
    limit, userID,
  )
  if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load logs"})
    return
  }
  defer rows.Close()

  var result []gin.H
  for rows.Next() {
    var id int
    var level, message string
    var createdAt sql.NullTime
    var domain sql.NullString
    if err := rows.Scan(&id, &level, &message, &createdAt, &domain); err != nil {
      c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse logs"})
      return
    }
    result = append(result, gin.H{
      "id":         id,
      "level":      level,
      "message":    message,
      "domain":     domain.String,
      "created_at": createdAt.Time,
    })
  }

  c.JSON(http.StatusOK, result)
}
