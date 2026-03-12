package middleware

import (
  "net/http"
  "strings"

  "domainguard/internal/auth"
  "github.com/gin-gonic/gin"
)

const userIDKey = "user_id"

func AuthMiddleware(secret string) gin.HandlerFunc {
  return func(c *gin.Context) {
    header := c.GetHeader("Authorization")
    if header == "" {
      c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
      return
    }

    parts := strings.SplitN(header, " ", 2)
    if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
      c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header"})
      return
    }

    claims, err := auth.ParseToken(parts[1], secret)
    if err != nil {
      c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
      return
    }

    c.Set(userIDKey, claims.UserID)
    c.Next()
  }
}

func GetUserID(c *gin.Context) int {
  value, exists := c.Get(userIDKey)
  if !exists {
    return 0
  }
  if id, ok := value.(int); ok {
    return id
  }
  return 0
}
