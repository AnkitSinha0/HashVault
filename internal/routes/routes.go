package routes

import (
	"github.com/AnkitSinha0/HashVault/internal/handlers"
	"github.com/AnkitSinha0/HashVault/internal/middleware"
	appjwt "github.com/AnkitSinha0/HashVault/pkg/jwt"
	"github.com/gin-gonic/gin"
)

type Handlers struct {
	Health *handlers.HealthHandler
	Auth   *handlers.AuthHandler
	OAuth  *handlers.OAuthHandler
}

func Setup(r *gin.Engine, h Handlers, jwt *appjwt.Manager) {
	r.GET("/health", h.Health.Check)

	v1 := r.Group("/api/v1")

	// Public auth routes — no JWT required.
	auth := v1.Group("/auth")
	{
		auth.POST("/register", h.Auth.Register)
		auth.POST("/login", h.Auth.Login)
		auth.POST("/refresh", h.Auth.Refresh)
		auth.POST("/logout", h.Auth.Logout)

		// OAuth2 — Google
		auth.GET("/google", h.OAuth.GoogleRedirect)
		auth.GET("/google/callback", h.OAuth.GoogleCallback)
	}

	// All protected routes live here — RequireAuth validates the Bearer token.
	// Phases 2–5 will add folder, file, and share routes to this group.
	protected := v1.Group("/", middleware.RequireAuth(jwt))
	_ = protected
}
