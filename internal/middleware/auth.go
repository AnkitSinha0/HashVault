package middleware

import (
	"net/http"
	"strings"

	appjwt "github.com/AnkitSinha0/HashVault/pkg/jwt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Context keys — import these in handlers to read user identity.
const (
	CtxUserID = "userID"
	CtxEmail  = "email"
)


// RequireAuth validates the Bearer token and injects userID + email into the
// gin context. Returns 401 if the token is missing, malformed, or expired.
func RequireAuth(jwtManager *appjwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or malformed Authorization header"})
			return
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims, err := jwtManager.ValidateAccessToken(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxEmail, claims.Email)
		c.Next()
	}
}

// MustGetUserID extracts the authenticated user's UUID from the gin context.
// Panics if called outside a RequireAuth-protected route — that's a programming error.
func MustGetUserID(c *gin.Context) uuid.UUID {
	return c.MustGet(CtxUserID).(uuid.UUID)
}