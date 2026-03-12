package auth

import (
  "time"

  "github.com/golang-jwt/jwt/v5"
)

type Claims struct {
  UserID int `json:"user_id"`
  jwt.RegisteredClaims
}

func GenerateToken(userID int, secret string, ttl time.Duration) (string, error) {
  claims := Claims{
    UserID: userID,
    RegisteredClaims: jwt.RegisteredClaims{
      ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
      IssuedAt:  jwt.NewNumericDate(time.Now()),
      Subject:   "user",
    },
  }

  token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
  return token.SignedString([]byte(secret))
}

func ParseToken(tokenString, secret string) (*Claims, error) {
  token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
    return []byte(secret), nil
  })
  if err != nil {
    return nil, err
  }

  claims, ok := token.Claims.(*Claims)
  if !ok || !token.Valid {
    return nil, jwt.ErrTokenInvalidClaims
  }
  return claims, nil
}
