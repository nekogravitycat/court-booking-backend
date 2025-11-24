package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid or expired token")
	ErrSignFailed   = errors.New("failed to sign token")
)

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
func (m *JWTManager) GenerateAccessToken(userID string) (string, error) {
	now := time.Now().UTC()

	claims := &jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", ErrSignFailed
	}

	return signed, nil
}

// ParseAndValidate validates a JWT and returns the parsed claims.
func (m *JWTManager) ParseAndValidate(tokenStr string) (*jwt.RegisteredClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		// Ensure token is signed using HS256
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
