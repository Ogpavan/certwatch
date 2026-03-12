package projects

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

type createRequest struct {
  Name string `json:"name" binding:"required"`
}

func (h *Handler) List(c *gin.Context) {
  userID := middleware.GetUserID(c)
  rows, err := h.DB.Query("SELECT id, user_id, name, created_at FROM projects WHERE user_id=@p1 ORDER BY created_at DESC", userID)
  if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch projects"})
    return
  }
  defer rows.Close()

  var result []gin.H
  for rows.Next() {
    var id, uid int
    var name string
    var createdAt sql.NullTime
    if err := rows.Scan(&id, &uid, &name, &createdAt); err != nil {
      c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse projects"})
      return
    }
    result = append(result, gin.H{
      "id":         id,
      "user_id":    uid,
      "name":       name,
      "created_at": createdAt.Time,
    })
  }

  c.JSON(http.StatusOK, result)
}

func (h *Handler) Create(c *gin.Context) {
  userID := middleware.GetUserID(c)
  var req createRequest
  if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
  }

  var id int
  err := h.DB.QueryRow(
    "INSERT INTO projects (user_id, name) OUTPUT INSERTED.id VALUES (@p1, @p2)",
    userID, req.Name,
  ).Scan(&id)
  if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create project"})
    return
  }

  c.JSON(http.StatusCreated, gin.H{"id": id, "name": req.Name})
}

func (h *Handler) Delete(c *gin.Context) {
  userID := middleware.GetUserID(c)
  id, err := strconv.Atoi(c.Param("id"))
  if err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
    return
  }

  res, err := h.DB.Exec("DELETE FROM projects WHERE id=@p1 AND user_id=@p2", id, userID)
  if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete project"})
    return
  }

  if rows, _ := res.RowsAffected(); rows == 0 {
    c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
    return
  }

  c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
