package handlers

import (
	"errors"
	"net/http"

	"github.com/AnkitSinha0/HashVault/internal/repositories"
	"github.com/AnkitSinha0/HashVault/internal/services"
	"github.com/gin-gonic/gin"
)

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"data": data})
}

func respondCreated(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, gin.H{"data": data})
}

func respondError(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": msg})
}

// respondServiceError maps well-known service/repo sentinel errors to HTTP
// status codes so every handler can call one function instead of a switch.
func respondServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrEmailTaken):
		respondError(c, http.StatusConflict, err.Error())
	case errors.Is(err, services.ErrInvalidCredentials):
		respondError(c, http.StatusUnauthorized, err.Error())
	case errors.Is(err, services.ErrInvalidToken):
		respondError(c, http.StatusUnauthorized, err.Error())
	case errors.Is(err, services.ErrStorageExceeded):
		respondError(c, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, repositories.ErrNotFound):
		respondError(c, http.StatusNotFound, "not found")
	default:
		respondError(c, http.StatusInternalServerError, "internal server error")
	}
}
