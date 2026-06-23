package config

import (
	"fmt"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	JWT      JWTConfig
	S3       S3Config
	RabbitMQ RabbitMQConfig
	OAuth    OAuthConfig
}

type ServerConfig struct {
	Port string
	Env  string
}

type DatabaseConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime int // seconds
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type JWTConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessTTL     int // minutes
	RefreshTTL    int // days
}

type S3Config struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	Region          string
	UseSSL          bool
}

type RabbitMQConfig struct {
	URL string
}

type OAuthConfig struct {
	GoogleClientID     string
	GoogleClientSecret string
	GoogleCallbackURL  string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config.yaml: %w", err)
	}


	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))


	cfg := &Config{
		Server: ServerConfig{
			Port: viper.GetString("server.port"),
			Env:  viper.GetString("server.env"),
		},
		Database: DatabaseConfig{
			DSN:             viper.GetString("database.dsn"),
			MaxOpenConns:    viper.GetInt("database.max_open_conns"),
			MaxIdleConns:    viper.GetInt("database.max_idle_conns"),
			ConnMaxLifetime: viper.GetInt("database.conn_max_lifetime"),
		},
		Redis: RedisConfig{
			Addr:     viper.GetString("redis.addr"),
			Password: viper.GetString("redis.password"),
			DB:       viper.GetInt("redis.db"),
		},
		JWT: JWTConfig{
			AccessSecret:  viper.GetString("jwt.access_secret"),
			RefreshSecret: viper.GetString("jwt.refresh_secret"),
			AccessTTL:     viper.GetInt("jwt.access_ttl"),
			RefreshTTL:    viper.GetInt("jwt.refresh_ttl"),
		},
		S3: S3Config{
			Endpoint:        viper.GetString("s3.endpoint"),
			AccessKeyID:     viper.GetString("s3.access_key_id"),
			SecretAccessKey: viper.GetString("s3.secret_access_key"),
			Bucket:          viper.GetString("s3.bucket"),
			Region:          viper.GetString("s3.region"),
			UseSSL:          viper.GetBool("s3.use_ssl"),
		},
		RabbitMQ: RabbitMQConfig{
			URL: viper.GetString("rabbitmq.url"),
		},
		OAuth: OAuthConfig{
			GoogleClientID:     viper.GetString("oauth.google_client_id"),
			GoogleClientSecret: viper.GetString("oauth.google_client_secret"),
			GoogleCallbackURL:  viper.GetString("oauth.google_callback_url"),
		},
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func validate(cfg *Config) error {
	if cfg.Database.DSN == "" {
		return fmt.Errorf("DATABASE_DSN is required")
	}
	if cfg.JWT.AccessSecret == "" {
		return fmt.Errorf("JWT_ACCESS_SECRET is required")
	}
	if cfg.JWT.RefreshSecret == "" {
		return fmt.Errorf("JWT_REFRESH_SECRET is required")
	}
	return nil
}
