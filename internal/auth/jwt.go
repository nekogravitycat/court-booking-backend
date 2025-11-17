package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims defines the JWT claims we embed in our token.
type Claims struct {
	UserID string `json:"sub"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// JWTManager manages JWT access token creation and validation.
type JWTManager struct {
	secret []byte
	ttl    time.Duration
}

// NewJWTManager creates a new JWT manager.
func NewJWTManager(secret string, ttl time.Duration) *JWTManager {
	return &JWTManager{
		secret: []byte(secret),
		ttl:    ttl,
	}
}

// GenerateAccessToken creates a signed JWT for the given user.
func (m *JWTManager) GenerateAccessToken(userID, email string) (string, error) {
	now := time.Now().UTC()

	claims := &Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("failed to sign jwt: %w", err)
	}

	return signed, nil
}

// ParseAndValidate validates a JWT and returns the parsed claims.
func (m *JWTManager) ParseAndValidate(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		// Ensure token is signed using HS256
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %T", t.Method)
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse jwt: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid jwt token")
	}

	return claims, nil
}
