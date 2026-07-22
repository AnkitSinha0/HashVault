package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/AnkitSinha0/HashVault/internal/config"
	"github.com/AnkitSinha0/HashVault/internal/dto"
	"github.com/AnkitSinha0/HashVault/internal/models"
	"github.com/AnkitSinha0/HashVault/internal/repositories"
	appjwt "github.com/AnkitSinha0/HashVault/pkg/jwt"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type OAuthService interface {
	// GoogleAuthURL generates a Google consent URL and returns it with the
	// opaque state token that the client must echo back on callback.
	GoogleAuthURL(ctx context.Context) (url, state string, err error)
	// GoogleCallback exchanges the authorization code for a user profile,
	// creates or finds the user, and issues a HashVault JWT pair.
	GoogleCallback(ctx context.Context, code, state string) (*dto.AuthResponse, error)
}

type oauthService struct {
	googleCfg *oauth2.Config
	users     repositories.UserRepository
	redis     *redis.Client
	jwt       *appjwt.Manager
}

func NewOAuthService(
	cfg *config.Config,
	users repositories.UserRepository,
	redis *redis.Client,
	jwt *appjwt.Manager,
) OAuthService {
	googleCfg := &oauth2.Config{
		ClientID:     cfg.OAuth.GoogleClientID,
		ClientSecret: cfg.OAuth.GoogleClientSecret,
		RedirectURL:  cfg.OAuth.GoogleCallbackURL,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}
	return &oauthService{
		googleCfg: googleCfg,
		users:     users,
		redis:     redis,
		jwt:       jwt,
	}
}

func (s *oauthService) GoogleAuthURL(ctx context.Context) (string, string, error) {
	// Generate random state to prevent CSRF on the callback.
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	state := hex.EncodeToString(b)

	// Store state in Redis for 10 minutes — callback must arrive within that window.
	if err := s.redis.Set(ctx, oauthStateKey(state), "1", 10*time.Minute).Err(); err != nil {
		return "", "", fmt.Errorf("storing oauth state: %w", err)
	}

	url := s.googleCfg.AuthCodeURL(state, oauth2.AccessTypeOnline)
	return url, state, nil
}

func (s *oauthService) GoogleCallback(ctx context.Context, code, state string) (*dto.AuthResponse, error) {
	// Validate state exists in Redis (CSRF check).
	key := oauthStateKey(state)
	if err := s.redis.Get(ctx, key).Err(); err != nil {
		return nil, ErrInvalidToken
	}
	s.redis.Del(ctx, key)

	token, err := s.googleCfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchanging code: %w", err)
	}

	profile, err := fetchGoogleProfile(ctx, s.googleCfg.Client(ctx, token))
	if err != nil {
		return nil, fmt.Errorf("fetching google profile: %w", err)
	}

	// Find existing user or create one (upsert pattern for OAuth).
	user, err := s.users.FindByEmail(ctx, profile.Email)
	if err != nil {
		if err != repositories.ErrNotFound {
			return nil, fmt.Errorf("looking up user: %w", err)
		}
		// New user — create without a password hash (OAuth-only account).
		user = &models.User{
			Name:         profile.Name,
			Email:        profile.Email,
			PasswordHash: "", // no password — login only via Google
		}
		if err := s.users.Create(ctx, user); err != nil {
			return nil, fmt.Errorf("creating oauth user: %w", err)
		}
	}

	accessToken, err := s.jwt.GenerateAccessToken(user.ID, user.Email)
	if err != nil {
		return nil, err
	}
	rawRefresh, err := s.jwt.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}
	if err := s.redis.Set(ctx, refreshKey(rawRefresh), user.ID.String(), s.jwt.RefreshTTL()).Err(); err != nil {
		return nil, err
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

type googleProfile struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

func fetchGoogleProfile(ctx context.Context, client *http.Client) (*googleProfile, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://www.googleapis.com/oauth2/v2/userinfo", nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var profile googleProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		return nil, err
	}
	return &profile, nil
}

func oauthStateKey(state string) string { return "oauth_state:" + state }
