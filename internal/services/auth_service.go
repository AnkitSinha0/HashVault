package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/AnkitSinha0/HashVault/internal/dto"
	"github.com/AnkitSinha0/HashVault/internal/models"
	"github.com/AnkitSinha0/HashVault/internal/queue"
	"github.com/AnkitSinha0/HashVault/internal/repositories"
	appjwt "github.com/AnkitSinha0/HashVault/pkg/jwt"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type AuthService interface {
	Register(ctx context.Context, req *dto.RegisterRequest) (*dto.AuthResponse, error)
	Login(ctx context.Context, req *dto.LoginRequest) (*dto.AuthResponse, error)
	Refresh(ctx context.Context, rawRefreshToken string) (*dto.AuthResponse, error)
	Logout(ctx context.Context, rawRefreshToken string) error
}

type authService struct {
	users     repositories.UserRepository
	redis     *redis.Client
	jwt       *appjwt.Manager
	publisher *queue.Publisher
}

func NewAuthService(
	users repositories.UserRepository,
	redis *redis.Client,
	jwt *appjwt.Manager,
	publisher *queue.Publisher,
) AuthService {
	return &authService{
		users:     users,
		redis:     redis,
		jwt:       jwt,
		publisher: publisher,
	}
}

func (s *authService) Register(ctx context.Context, req *dto.RegisterRequest) (*dto.AuthResponse, error) {
	// Check email uniqueness before hashing — avoids bcrypt cost on duplicates.
	if _, err := s.users.FindByEmail(ctx, req.Email); !errors.Is(err, repositories.ErrNotFound) {
		if err == nil {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("checking email: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	user := &models.User{
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(hash),
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("creating user: %w", err)
	}

	resp, err := s.issueTokenPair(ctx, user)
	if err != nil {
		return nil, err
	}

	// Fire-and-forget — registration succeeds even if the queue is down.
	s.publisher.Publish(ctx, queue.EventWelcomeEmail, queue.WelcomeEmailPayload{
		UserID: user.ID.String(),
		Name:   user.Name,
		Email:  user.Email,
	})

	return resp, nil
}

func (s *authService) Login(ctx context.Context, req *dto.LoginRequest) (*dto.AuthResponse, error) {
	user, err := s.users.FindByEmail(ctx, req.Email)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("finding user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.issueTokenPair(ctx, user)
}

func (s *authService) Refresh(ctx context.Context, rawToken string) (*dto.AuthResponse, error) {
	key := refreshKey(rawToken)

	userIDStr, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		// redis.Nil means token not found (expired or already rotated).
		return nil, ErrInvalidToken
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// Delete before issuing new token — rotation prevents replay attacks.
	// If this fails, old token remains valid until TTL; acceptable trade-off.
	s.redis.Del(ctx, key)

	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("finding user: %w", err)
	}

	return s.issueTokenPair(ctx, user)
}

func (s *authService) Logout(ctx context.Context, rawToken string) error {
	s.redis.Del(ctx, refreshKey(rawToken))
	return nil
}

// issueTokenPair generates a JWT access token and a random refresh token,
// stores the refresh token hash in Redis, and returns the auth response.
func (s *authService) issueTokenPair(ctx context.Context, user *models.User) (*dto.AuthResponse, error) {
	accessToken, err := s.jwt.GenerateAccessToken(user.ID, user.Email)
	if err != nil {
		return nil, fmt.Errorf("generating access token: %w", err)
	}

	rawRefresh, err := s.jwt.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generating refresh token: %w", err)
	}

	if err := s.redis.Set(ctx, refreshKey(rawRefresh), user.ID.String(), s.jwt.RefreshTTL()).Err(); err != nil {
		return nil, fmt.Errorf("storing refresh token: %w", err)
	}

	return &dto.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		User: dto.UserDTO{
			ID:           user.ID,
			Name:         user.Name,
			Email:        user.Email,
			StorageLimit: user.StorageLimit,
			UsedStorage:  user.UsedStorage,
		},
	}, nil
}

// refreshKey hashes the raw token so the plaintext is never stored in Redis.
// Key format: refresh_token:{sha256hex}
func refreshKey(rawToken string) string {
	h := sha256.Sum256([]byte(rawToken))
	return "refresh_token:" + hex.EncodeToString(h[:])
}

// Ensure the compiler catches drift between interface and struct.
var _ AuthService = (*authService)(nil)

// Keep time import used via jwt TTL internals.
var _ = time.Second
