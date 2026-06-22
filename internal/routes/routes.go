package routes

import (
	"github.com/AnkitSinha0/HashVault/internal/handlers"
	"github.com/gin-gonic/gin"
)

func Setup(r *gin.Engine) {
	health := handlers.NewHealthHandler()
	r.GET("/health", health.Check)

	v1 := r.Group("/api/v1")
	_ = v1
}
