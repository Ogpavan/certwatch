package auth

import (
  "database/sql"
  "log"
  "net/http"
  "strings"
  "time"

  "github.com/gin-gonic/gin"
  "golang.org/x/crypto/bcrypt"
)

type Handler struct {
  DB        *sql.DB
  JWTSecret string
  TokenTTL  time.Duration
}

type authRequest struct {
  Email    string `json:"email" binding:"required,email"`
  Password string `json:"password" binding:"required,min=6"`
}

type authResponse struct {
  Token string `json:"token"`
}

func (h *Handler) Register(c *gin.Context) {
  var req authRequest
  if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
  }

  email := strings.ToLower(strings.TrimSpace(req.Email))
  hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
  if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
    return
  }

  var userID int
  err = h.DB.QueryRow(
    "INSERT INTO users (email, password_hash) OUTPUT INSERTED.id VALUES (@p1, @p2)",
    email, string(hash),
  ).Scan(&userID)
  if err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": "email already exists"})
    return
  }

  token, err := GenerateToken(userID, h.JWTSecret, h.TokenTTL)
  if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
    return
  }

  if _, err := h.DB.Exec(
    `INSERT INTO user_notification_settings (user_id, email_enabled, email_recipients, notify_days, schedule_time)
     VALUES (@p1, 1, '[]', '["30","14","7","3"]', '03:00')`,
    userID,
  ); err != nil {
    log.Printf("auth: failed to insert default settings for user %d: %v", userID, err)
  }

  c.JSON(http.StatusCreated, authResponse{Token: token})
}

func (h *Handler) Login(c *gin.Context) {
  var req authRequest
  if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
  }

  email := strings.ToLower(strings.TrimSpace(req.Email))
  var userID int
  var hash string
  err := h.DB.QueryRow("SELECT id, password_hash FROM users WHERE email=@p1", email).Scan(&userID, &hash)
  if err != nil {
    c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
    return
  }

  if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
    c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
    return
  }

  token, err := GenerateToken(userID, h.JWTSecret, h.TokenTTL)
  if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
    return
  }

  c.JSON(http.StatusOK, authResponse{Token: token})
}
