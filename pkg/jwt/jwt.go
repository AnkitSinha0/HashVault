package jwt

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken  = errors.New("token expired")
)

type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	jwt.RegisteredClaims
}

type Manager struct {
	accessSecret []byte
	accessTTL    time.Duration
	refreshTTL   time.Duration
}

func NewManager(accessSecret string, accessTTL, refreshTTL time.Duration) *Manager {
	return &Manager{
		accessSecret: []byte(accessSecret),
		accessTTL:    accessTTL,
		refreshTTL:   refreshTTL,
	}
}

func (m *Manager) GenerateAccessToken(userID uuid.UUID, email string) (string, error) {
	claims := Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.accessTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "hashvault",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.accessSecret)
}

// GenerateRefreshToken returns a cryptographically random 64-char hex string.
// It carries no payload — it's an opaque pointer into Redis, not a signed JWT.
func (m *Manager) GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (m *Manager) ValidateAccessToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.accessSecret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}
	// type assertion 
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func (m *Manager) RefreshTTL() time.Duration { return m.refreshTTL }
