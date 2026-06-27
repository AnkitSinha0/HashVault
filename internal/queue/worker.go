package queue

import (
	"context"
	"encoding/json"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

const queueName = "hashvault.email"

type Worker struct {
	conn *amqp.Connection
	log  *zap.Logger
}

func NewWorker(conn *amqp.Connection, log *zap.Logger) *Worker {
	return &Worker{conn: conn, log: log}
}

// Start binds the queue to the exchange and begins consuming in a goroutine.
// The goroutine exits when ctx is cancelled (graceful shutdown).
func (w *Worker) Start(ctx context.Context) error {
	ch, err := w.conn.Channel()
	if err != nil {
		return err
	}

	q, err := ch.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		return err
	}

	for _, key := range []string{EventWelcomeEmail, EventOTPEmail} {
		if err := ch.QueueBind(q.Name, key, exchangeName, false, nil); err != nil {
			return err
		}
	}

	// Prefetch 1: process and ack one message before fetching the next.
	// Prevents a slow consumer from holding all messages while idle.
	_ = ch.Qos(1, 0, false)

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		return err
	}

	go w.loop(ctx, msgs, ch)
	w.log.Info("queue worker started", zap.String("queue", queueName))
	return nil
}

func (w *Worker) loop(ctx context.Context, msgs <-chan amqp.Delivery, ch *amqp.Channel) {
	defer ch.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgs:
			if !ok {
				w.log.Warn("queue channel closed unexpectedly")
				return
			}
			w.handle(msg)
		}
	}
}

func (w *Worker) handle(msg amqp.Delivery) {
	switch msg.RoutingKey {
	case EventWelcomeEmail:
		var p WelcomeEmailPayload
		if err := json.Unmarshal(msg.Body, &p); err != nil {
			w.log.Error("worker: bad welcome payload", zap.Error(err))
			_ = msg.Nack(false, false) // don't requeue malformed messages
			return
		}
		// TODO Phase 6: replace with real SMTP call
		w.log.Info("worker: sending welcome email", zap.String("to", p.Email), zap.String("name", p.Name))

	case EventOTPEmail:
		var p OTPEmailPayload
		if err := json.Unmarshal(msg.Body, &p); err != nil {
			w.log.Error("worker: bad OTP payload", zap.Error(err))
			_ = msg.Nack(false, false)
			return
		}
		// TODO Phase 6: replace with real SMTP call
		w.log.Info("worker: sending OTP email", zap.String("to", p.Email))

	default:
		w.log.Warn("worker: unknown routing key", zap.String("key", msg.RoutingKey))
	}
	_ = msg.Ack(false)
}
