package queue

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const exchangeName = "hashvault.events"

type Publisher struct {
	ch  *amqp.Channel
	log *zap.Logger
}

func NewPublisher(conn *amqp.Connection, log *zap.Logger) (*Publisher, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("opening channel: %w", err)
	}

	// Topic exchange: routes by routing key pattern (email.*, file.*, etc.)
	// Durable: exchange survives broker restart.
	if err := ch.ExchangeDeclare(
		exchangeName,
		"topic",
		true,  // durable
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,
	); err != nil {
		return nil, fmt.Errorf("declaring exchange: %w", err)
	}

	return &Publisher{ch: ch, log: log}, nil
}

// Publish sends payload as a persistent JSON message to the exchange.
// Errors are logged but not returned — email delivery is non-critical and
// must never roll back the caller's user operation.
func (p *Publisher) Publish(ctx context.Context, routingKey string, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		p.log.Error("queue: marshal failed", zap.String("event", routingKey), zap.Error(err))
		return
	}

	if err := p.ch.PublishWithContext(ctx,
		exchangeName,
		routingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent, // message survives broker restart
			Body:         body,
		},
	); err != nil {
		p.log.Error("queue: publish failed", zap.String("event", routingKey), zap.Error(err))
	}
}
