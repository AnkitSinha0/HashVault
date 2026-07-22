package handlers

import (
	"net/http"

	"github.com/AnkitSinha0/HashVault/internal/dto"
	"github.com/AnkitSinha0/HashVault/internal/services"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	auth services.AuthService
}

func NewAuthHandler(auth services.AuthService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

// Register godoc
// POST /api/v1/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := h.auth.Register(c.Request.Context(), &req)
	if err != nil {
		respondServiceError(c, err)
		return
	}

	respondCreated(c, resp)
}

// Login godoc
// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := h.auth.Login(c.Request.Context(), &req)
	if err != nil {
		respondServiceError(c, err)
		return
	}

	respondOK(c, resp)
}

// Refresh godoc
// POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req dto.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := h.auth.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		respondServiceError(c, err)
		return
	}

	respondOK(c, resp)
}

// Logout godoc
// POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c *gin.Context) {
	var req dto.LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	_ = h.auth.Logout(c.Request.Context(), req.RefreshToken)
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}
