package alerts

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
  rows, err := h.DB.Query(
    `SELECT a.id, a.domain_id, a.type, a.severity, a.message, a.created_at, a.resolved
     FROM alerts a
     INNER JOIN domains d ON a.domain_id = d.id
     INNER JOIN projects p ON d.project_id = p.id
     WHERE p.user_id=@p1
     ORDER BY a.created_at DESC`,
    userID,
  )
  if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch alerts"})
    return
  }
  defer rows.Close()

  var result []gin.H
  for rows.Next() {
    var id, domainID int
    var alertType, severity, message sql.NullString
    var createdAt sql.NullTime
    var resolved bool
    if err := rows.Scan(&id, &domainID, &alertType, &severity, &message, &createdAt, &resolved); err != nil {
      c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse alerts"})
      return
    }
    result = append(result, gin.H{
      "id":         id,
      "domain_id":  domainID,
      "type":       alertType.String,
      "severity":   severity.String,
      "message":    message.String,
      "created_at": createdAt.Time,
      "resolved":   resolved,
    })
  }

  c.JSON(http.StatusOK, result)
}

func (h *Handler) Resolve(c *gin.Context) {
  userID := middleware.GetUserID(c)
  id, err := strconv.Atoi(c.Param("id"))
  if err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alert id"})
    return
  }

  res, err := h.DB.Exec(
    `UPDATE a SET resolved=1
     FROM alerts a
     INNER JOIN domains d ON a.domain_id = d.id
     INNER JOIN projects p ON d.project_id = p.id
     WHERE a.id=@p1 AND p.user_id=@p2`,
    id, userID,
  )
  if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve alert"})
    return
  }

  if rows, _ := res.RowsAffected(); rows == 0 {
    c.JSON(http.StatusNotFound, gin.H{"error": "alert not found"})
    return
  }

  c.JSON(http.StatusOK, gin.H{"status": "resolved"})
}
