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

func Load() (*Config, error) {
	// Step 1: load .env into real OS environment variables (development only).
	// godotenv.Load() is intentionally silent on a missing file — in production
	// there is no .env file; secrets arrive as real env vars from Docker/K8s/AWS.
	_ = godotenv.Load()

	// Step 2: read config.yaml (non-secret, committed to git).
	// Viper reads nested YAML keys like `database.max_open_conns`.
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config.yaml: %w", err)
	}

	// Step 3: environment variables override everything in the YAML.
	// SetEnvKeyReplacer maps nested yaml keys to flat env var names:
	//   database.dsn  →  DATABASE_DSN
	//   s3.access_key_id  →  S3_ACCESS_KEY_ID
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// --- ALTERNATIVE: viper.Unmarshal ---
	// Instead of filling cfg field-by-field, viper can decode the entire merged
	// config (yaml + env vars) into a struct in one call:
	//
	//   var cfg Config
	//   if err := viper.Unmarshal(&cfg); err != nil {
	//       return nil, fmt.Errorf("unmarshaling config: %w", err)
	//   }
	//
	// Unmarshal uses the `mapstructure` library internally. It lowercases Go
	// field names, so "DSN" → "dsn" and "MaxOpenConns" → "maxopenconns", which
	// won't match the yaml key "max_open_conns". To fix that, add mapstructure
	// tags to every struct field:
	//
	//   type DatabaseConfig struct {
	//       DSN             string `mapstructure:"dsn"`
	//       MaxOpenConns    int    `mapstructure:"max_open_conns"`
	//       ...
	//   }
	//
	// Less code in Load() but more noise on every struct. The explicit
	// GetString/GetInt approach below makes each yaml→Go mapping obvious.
	// Both patterns are used in production; this one is clearer while learning.
	// -------------------------------------------------------------------------

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
