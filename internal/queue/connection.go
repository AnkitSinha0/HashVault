package queue

import (
	"fmt"

	"github.com/AnkitSinha0/HashVault/internal/config"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

func NewConnection(cfg *config.Config, log *zap.Logger) (*amqp.Connection, error) {
	conn, err := amqp.Dial(cfg.RabbitMQ.URL)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq dial: %w", err)
	}
	log.Info("rabbitmq connected")
	return conn, nil
}
