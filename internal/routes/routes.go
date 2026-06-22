package routes

import (
	"github.com/AnkitSinha0/HashVault/internal/handlers"
	"github.com/gin-gonic/gin"
)

func Setup(r *gin.Engine) {
	health := handlers.NewHealthHandler()
	r.GET("/health", health.Check)

	// All application routes live under /api/v1.
	// Each phase adds its own group here (auth, users, folders, files, share).
	v1 := r.Group("/api/v1")
	_ = v1
}
