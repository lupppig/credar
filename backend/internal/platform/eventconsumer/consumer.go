// Package eventconsumer provides the infrastructure-only durable consumer used
// to prove JetStream redelivery and database idempotency.
package eventconsumer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lupppig/credar/backend/internal/platform/database"
	"github.com/lupppig/credar/backend/internal/platform/event"
	"github.com/lupppig/credar/backend/internal/platform/natsclient"
	"github.com/nats-io/nats.go"
)

const (
	// InfrastructureConsumer is deliberately scoped to the Phase 01 proof
	// event and must not become a catch-all domain-event consumer.
	InfrastructureConsumer = "credar-infrastructure-proof-v1"
	InfrastructureSubject  = "credar.events.infrastructure.probe.v1"
	deadLetterSubject      = "credar.dead.infrastructure.proof.v1"
	maxDeliveries          = 5
)

// Consumer records exactly one durable result per proof event ID.
type Consumer struct {
	database *database.Database
	nats     *natsclient.Client
	logger   *slog.Logger

	mu           sync.RWMutex
	cancel       context.CancelFunc
	done         chan struct{}
	subscription *nats.Subscription
	lastError    error
}

// New constructs the infrastructure proof consumer.
func New(
	databaseConnection *database.Database,
	natsConnection *natsclient.Client,
	logger *slog.Logger,
) (*Consumer, error) {
	if databaseConnection == nil || natsConnection == nil || logger == nil {
		return nil, fmt.Errorf("event consumer database, NATS client, and logger are required")
	}
	return &Consumer{database: databaseConnection, nats: natsConnection, logger: logger}, nil
}

// Name identifies the worker in readiness output.
func (*Consumer) Name() string {
	return "event-consumer"
}

// Start creates or updates the durable consumer and begins pull processing.
func (consumer *Consumer) Start(parent context.Context) error {
	consumer.mu.Lock()
	defer consumer.mu.Unlock()
	if consumer.cancel != nil {
		return fmt.Errorf("event consumer has already started")
	}
	jetStream, err := consumer.nats.JetStream()
	if err != nil {
		return err
	}
	if err := ensureConsumer(jetStream); err != nil {
		return err
	}
	subscription, err := jetStream.PullSubscribe(
		InfrastructureSubject,
		InfrastructureConsumer,
		nats.Bind(natsclient.EventsStream, InfrastructureConsumer),
	)
	if err != nil {
		return fmt.Errorf("bind infrastructure proof consumer: %w", err)
	}
	ctx, cancel := context.WithCancel(parent)
	consumer.cancel = cancel
	consumer.done = make(chan struct{})
	consumer.subscription = subscription
	go consumer.run(ctx, consumer.done, subscription)
	consumer.logger.Info("infrastructure event consumer started", "consumer", InfrastructureConsumer)
	return nil
}

// Check reports loop failures; connection health remains owned by natsclient.
func (consumer *Consumer) Check(context.Context) error {
	consumer.mu.RLock()
	defer consumer.mu.RUnlock()
	if consumer.cancel == nil {
		return fmt.Errorf("event consumer is not started")
	}
	return consumer.lastError
}

// Close stops fetching and removes only the local subscription binding. The
// durable server-side consumer remains for redelivery after restart.
func (consumer *Consumer) Close(ctx context.Context) error {
	consumer.mu.Lock()
	cancel := consumer.cancel
	done := consumer.done
	subscription := consumer.subscription
	consumer.cancel = nil
	consumer.done = nil
	consumer.subscription = nil
	consumer.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	if subscription != nil {
		subscription.Unsubscribe()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stop event consumer: %w", ctx.Err())
	}
}

func (consumer *Consumer) run(
	ctx context.Context,
	done chan<- struct{},
	subscription *nats.Subscription,
) {
	defer close(done)
	for {
		if ctx.Err() != nil {
			return
		}
		messages, err := subscription.Fetch(10, nats.MaxWait(500*time.Millisecond))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) || errors.Is(err, context.Canceled) {
				consumer.setError(nil)
				continue
			}
			consumer.setError(err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		consumer.setError(nil)
		for _, message := range messages {
			consumer.process(ctx, message)
		}
	}
}

func (consumer *Consumer) process(ctx context.Context, message *nats.Msg) {
	metadata, metadataErr := message.Metadata()
	deliveryCount := uint64(1)
	if metadataErr == nil {
		deliveryCount = metadata.NumDelivered
	}

	var envelope event.Envelope
	if err := json.Unmarshal(message.Data, &envelope); err != nil || envelope.Validate() != nil {
		consumer.deadLetter(ctx, message, envelope.ID, deliveryCount, "invalid_event_envelope")
		return
	}
	attributes := []any{
		"consumer", InfrastructureConsumer,
		"event_id", envelope.ID,
		"event_type", envelope.Type,
		"aggregate_id", envelope.Aggregate.ID,
		"correlation_id", envelope.CorrelationID,
		"delivery_count", deliveryCount,
	}
	consumer.logger.InfoContext(ctx, "event consumption started", attributes...)

	err := consumer.database.InTransaction(ctx, pgx.TxOptions{}, func(transaction database.DBTX) error {
		_, err := transaction.Exec(
			ctx,
			`INSERT INTO platform.event_consumptions (
                consumer_name, event_id, event_type, delivery_count
            ) VALUES ($1, $2, $3, $4)
            ON CONFLICT (consumer_name, event_id) DO NOTHING`,
			InfrastructureConsumer,
			envelope.ID,
			envelope.Type,
			int64(deliveryCount),
		)
		return err
	})
	if err == nil {
		if ackErr := message.Ack(); ackErr != nil {
			consumer.setError(fmt.Errorf("acknowledge event: %w", ackErr))
			consumer.logger.WarnContext(
				ctx,
				"event acknowledgement failed",
				append(attributes, "error_code", "event_acknowledgement_failed")...,
			)
			return
		}
		consumer.logger.InfoContext(ctx, "event consumption completed", attributes...)
		return
	}

	if deliveryCount >= maxDeliveries {
		consumer.deadLetter(ctx, message, envelope.ID, deliveryCount, "consumer_processing_failed")
		return
	}
	if nakErr := message.NakWithDelay(redeliveryBackoff(deliveryCount)); nakErr != nil {
		consumer.setError(errors.Join(err, nakErr))
		return
	}
	consumer.setError(err)
	consumer.logger.WarnContext(
		ctx,
		"event consumption retry scheduled",
		append(attributes, "error_code", "consumer_processing_failed")...,
	)
}

func (consumer *Consumer) deadLetter(
	ctx context.Context,
	message *nats.Msg,
	eventID string,
	deliveryCount uint64,
	reasonCode string,
) {
	digest := sha256.Sum256(message.Data)
	representation := struct {
		Consumer      string    `json:"consumer"`
		EventID       string    `json:"event_id,omitempty"`
		Subject       string    `json:"subject"`
		DeliveryCount uint64    `json:"delivery_count"`
		ReasonCode    string    `json:"reason_code"`
		PayloadHash   string    `json:"payload_sha256"`
		FailedAt      time.Time `json:"failed_at"`
	}{
		Consumer:      InfrastructureConsumer,
		EventID:       eventID,
		Subject:       message.Subject,
		DeliveryCount: deliveryCount,
		ReasonCode:    reasonCode,
		PayloadHash:   hex.EncodeToString(digest[:]),
		FailedAt:      time.Now().UTC(),
	}
	payload, err := json.Marshal(representation)
	if err == nil {
		jetStream, jsErr := consumer.nats.JetStream()
		if jsErr == nil {
			_, err = jetStream.Publish(deadLetterSubject, payload, nats.Context(ctx))
		} else {
			err = jsErr
		}
	}
	if err != nil {
		consumer.setError(fmt.Errorf("publish consumer dead letter: %w", err))
		_ = message.NakWithDelay(time.Minute)
		return
	}
	if err := message.Term(); err != nil {
		consumer.setError(fmt.Errorf("terminate dead-lettered event: %w", err))
		return
	}
	consumer.logger.ErrorContext(
		ctx,
		"event consumption dead-lettered",
		"consumer", InfrastructureConsumer,
		"event_id", eventID,
		"subject", message.Subject,
		"delivery_count", deliveryCount,
		"error_code", reasonCode,
	)
}

func (consumer *Consumer) setError(err error) {
	consumer.mu.Lock()
	defer consumer.mu.Unlock()
	consumer.lastError = err
}

func ensureConsumer(jetStream nats.JetStreamContext) error {
	configuration := &nats.ConsumerConfig{
		Durable:       InfrastructureConsumer,
		AckPolicy:     nats.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    maxDeliveries,
		FilterSubject: InfrastructureSubject,
		DeliverPolicy: nats.DeliverAllPolicy,
		ReplayPolicy:  nats.ReplayInstantPolicy,
		MaxAckPending: 100,
	}
	if _, err := jetStream.ConsumerInfo(natsclient.EventsStream, InfrastructureConsumer); err != nil {
		if !errors.Is(err, nats.ErrConsumerNotFound) {
			return fmt.Errorf("inspect infrastructure consumer: %w", err)
		}
		if _, err := jetStream.AddConsumer(natsclient.EventsStream, configuration); err != nil {
			return fmt.Errorf("create infrastructure consumer: %w", err)
		}
		return nil
	}
	if _, err := jetStream.UpdateConsumer(natsclient.EventsStream, configuration); err != nil {
		return fmt.Errorf("update infrastructure consumer: %w", err)
	}
	return nil
}

func redeliveryBackoff(delivery uint64) time.Duration {
	backoff := time.Second
	for current := uint64(1); current < delivery && backoff < time.Minute; current++ {
		backoff *= 2
	}
	if backoff > time.Minute {
		return time.Minute
	}
	return backoff
}
