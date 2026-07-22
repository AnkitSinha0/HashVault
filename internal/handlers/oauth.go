package handlers

import (
	"net/http"

	"github.com/AnkitSinha0/HashVault/internal/services"
	"github.com/gin-gonic/gin"
)

type OAuthHandler struct {
	oauth services.OAuthService
}

func NewOAuthHandler(oauth services.OAuthService) *OAuthHandler {
	return &OAuthHandler{oauth: oauth}
}

// GoogleRedirect godoc
// GET /api/v1/auth/google
// Returns the Google consent URL. Client opens it in the browser.
func (h *OAuthHandler) GoogleRedirect(c *gin.Context) {
	url, state, err := h.oauth.GoogleAuthURL(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "could not generate auth URL")
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": url, "state": state})
}

// GoogleCallback godoc
// GET /api/v1/auth/google/callback
// Google redirects here after the user consents. Returns HashVault JWT pair.
func (h *OAuthHandler) GoogleCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" || state == "" {
		respondError(c, http.StatusBadRequest, "missing code or state")
		return
	}

	resp, err := h.oauth.GoogleCallback(c.Request.Context(), code, state)
	if err != nil {
		respondServiceError(c, err)
		return
	}

	respondOK(c, resp)
}
