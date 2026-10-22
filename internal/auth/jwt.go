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

// Claims are the access token claims. SessionID ties the token to a server-side
// session so that revoking the session invalidates the token immediately.
type Claims struct {
	jwt.RegisteredClaims
	SessionID string `json:"sid"`
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

// GenerateAccessToken creates a signed JWT for the given user and session.
func (m *JWTManager) GenerateAccessToken(userID, sessionID string) (string, error) {
	now := time.Now().UTC()

	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
		SessionID: sessionID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", ErrSignFailed
	}

	return signed, nil
}

// ParseAndValidate validates a JWT and returns the parsed claims.
func (m *JWTManager) ParseAndValidate(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid || claims.SessionID == "" {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
