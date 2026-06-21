package database

import (
	"fmt"
	"time"

	"github.com/AnkitSinha0/HashVault/internal/config"
	"github.com/AnkitSinha0/HashVault/internal/models"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func NewPostgresDB(cfg *config.Config, log *zap.Logger) (*gorm.DB, error) {
	gormLogLevel := gormlogger.Warn
	if cfg.Server.Env == "development" {
		gormLogLevel = gormlogger.Info
	}

	db, err := gorm.Open(postgres.Open(cfg.Database.DSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormLogLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("opening postgres connection: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("getting underlying sql.DB: %w", err)
	}

	// Tune the connection pool.
	// MaxOpenConns caps total connections to Postgres (default is unlimited — dangerous).
	// MaxIdleConns limits idle connections kept alive between requests.
	// ConnMaxLifetime prevents stale connections after a DB restart or network hiccup.
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetime) * time.Second)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	log.Info("connected to postgres",
		zap.Int("max_open_conns", cfg.Database.MaxOpenConns),
		zap.Int("max_idle_conns", cfg.Database.MaxIdleConns),
	)

	return db, nil
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.User{},
		&models.Folder{},
		&models.StorageObject{},
		&models.File{},
		&models.ShareLink{},
	)
}
