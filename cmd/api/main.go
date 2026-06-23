package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AnkitSinha0/HashVault/internal/config"
	"github.com/AnkitSinha0/HashVault/internal/database"
	"github.com/AnkitSinha0/HashVault/internal/routes"
	"github.com/AnkitSinha0/HashVault/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	logger.Init(cfg.Server.Env)
	defer logger.Sync()
	log := logger.Log

	db, err := database.NewPostgresDB(cfg, log)
	if err != nil {
		log.Fatal("database connection failed", zap.Error(err))
	}

	if err := database.AutoMigrate(db); err != nil {
		log.Fatal("database migration failed", zap.Error(err))
	}
	log.Info("database migrations complete")

	if cfg.Server.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())

	routes.Setup(r)

	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("server starting", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down — draining requests (30s timeout)")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("forced shutdown", zap.Error(err))
	}
	log.Info("server stopped cleanly")
}
