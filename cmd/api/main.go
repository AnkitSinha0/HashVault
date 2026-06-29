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
	"github.com/AnkitSinha0/HashVault/internal/handlers"
	"github.com/AnkitSinha0/HashVault/internal/queue"
	"github.com/AnkitSinha0/HashVault/internal/repositories"
	"github.com/AnkitSinha0/HashVault/internal/routes"
	"github.com/AnkitSinha0/HashVault/internal/services"
	appjwt "github.com/AnkitSinha0/HashVault/pkg/jwt"
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

	// --- Infrastructure ---

	db, err := database.NewPostgresDB(cfg, log)
	if err != nil {
		log.Fatal("database connection failed", zap.Error(err))
	}
	if err := database.AutoMigrate(db); err != nil {
		log.Fatal("database migration failed", zap.Error(err))
	}
	log.Info("database migrations complete")

	redisClient, err := database.NewRedisClient(cfg, log)
	if err != nil {
		log.Fatal("redis connection failed", zap.Error(err))
	}

	mqConn, err := queue.NewConnection(cfg, log)
	if err != nil {
		log.Fatal("rabbitmq connection failed", zap.Error(err))
	}
	defer mqConn.Close()

	publisher, err := queue.NewPublisher(mqConn, log)
	if err != nil {
		log.Fatal("failed to create queue publisher", zap.Error(err))
	}

	// --- Core dependencies ---

	jwtManager := appjwt.NewManager(
		cfg.JWT.AccessSecret,
		time.Duration(cfg.JWT.AccessTTL)*time.Minute,
		time.Duration(cfg.JWT.RefreshTTL)*24*time.Hour,
	)

	// --- Repositories ---

	userRepo := repositories.NewUserRepository(db)

	// --- Services ---

	authSvc := services.NewAuthService(userRepo, redisClient, jwtManager, publisher)
	oauthSvc := services.NewOAuthService(cfg, userRepo, redisClient, jwtManager)

	// --- Queue worker ---

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()

	worker := queue.NewWorker(mqConn, log)
	if err := worker.Start(workerCtx); err != nil {
		log.Fatal("failed to start queue worker", zap.Error(err))
	}

	// --- HTTP server ---

	if cfg.Server.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())

	routes.Setup(r, routes.Handlers{
		Health: handlers.NewHealthHandler(),
		Auth:   handlers.NewAuthHandler(authSvc),
		OAuth:  handlers.NewOAuthHandler(oauthSvc),
	}, jwtManager)

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
	cancelWorker()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("forced shutdown", zap.Error(err))
	}
	log.Info("server stopped cleanly")
}
