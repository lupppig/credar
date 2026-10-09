//go:build integration

package outbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lupppig/credar/backend/internal/platform/database"
	"github.com/lupppig/credar/backend/internal/platform/event"
	"github.com/lupppig/credar/backend/internal/platform/eventconsumer"
	"github.com/lupppig/credar/backend/internal/platform/identifier"
	"github.com/lupppig/credar/backend/internal/platform/natsclient"
	"github.com/lupppig/credar/backend/internal/platform/outbox"
	"github.com/nats-io/nats.go"
)

func TestCommittedOutboxEventPublishesAndConsumesIdempotently(t *testing.T) {
	databaseURL := os.Getenv("CREDAR_TEST_DATABASE_URL")
	natsURL := os.Getenv("CREDAR_TEST_NATS_URL")
	if databaseURL == "" || natsURL == "" {
		t.Skip("CREDAR_TEST_DATABASE_URL and CREDAR_TEST_NATS_URL are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	databaseConnection, err := database.New(database.Config{
		ConnectionString: databaseURL,
		MaxConnections:   5,
		MinConnections:   0,
	})
	if err != nil {
		t.Fatalf("construct database: %v", err)
	}
	if err := databaseConnection.Start(ctx); err != nil {
		t.Fatalf("start database: %v", err)
	}
	t.Cleanup(func() { _ = databaseConnection.Close(context.Background()) })

	natsConnection, err := natsclient.New(natsURL)
	if err != nil {
		t.Fatalf("construct NATS client: %v", err)
	}
	if err := natsConnection.Start(ctx); err != nil {
		t.Fatalf("start NATS client: %v", err)
	}
	t.Cleanup(func() { _ = natsConnection.Close(context.Background()) })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	consumer, err := eventconsumer.New(databaseConnection, natsConnection, logger)
	if err != nil {
		t.Fatalf("construct consumer: %v", err)
	}
	if err := consumer.Start(ctx); err != nil {
		t.Fatalf("start consumer: %v", err)
	}
	t.Cleanup(func() { _ = consumer.Close(context.Background()) })

	repository := outbox.NewRepository(databaseConnection)
	committed := integrationEnvelope(t, "committed")
	rolledBack := integrationEnvelope(t, "rolled_back")
	if err := databaseConnection.InTransaction(ctx, pgx.TxOptions{}, func(transaction database.DBTX) error {
		return repository.Insert(ctx, transaction, committed)
	}); err != nil {
		t.Fatalf("commit outbox event: %v", err)
	}

	rollbackCause := errors.New("rollback proof event")
	err = databaseConnection.InTransaction(ctx, pgx.TxOptions{}, func(transaction database.DBTX) error {
		if err := repository.Insert(ctx, transaction, rolledBack); err != nil {
			return err
		}
		return rollbackCause
	})
	if !errors.Is(err, rollbackCause) {
		t.Fatalf("expected rollback cause, got %v", err)
	}

	abandonedWorker, err := identifier.NewUUIDv7()
	if err != nil {
		t.Fatalf("create abandoned worker ID: %v", err)
	}
	claimed, err := repository.Claim(ctx, abandonedWorker, 1, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("claim proof event: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Envelope.ID != committed.ID {
		t.Fatalf("unexpected claimed events: %#v", claimed)
	}

	pool, err := databaseConnection.Pool()
	if err != nil {
		t.Fatalf("get database pool: %v", err)
	}
	waitFor(t, ctx, "outbox lease expiry", func() (bool, error) {
		var expired bool
		err := pool.QueryRow(
			ctx,
			`SELECT lease_expires_at <= now() FROM platform.outbox_events WHERE id = $1`,
			committed.ID,
		).Scan(&expired)
		return expired, err
	})

	relay, err := outbox.NewRelay(repository, natsConnection, outbox.RelayConfig{
		PollInterval:  20 * time.Millisecond,
		LeaseDuration: time.Second,
		BatchSize:     10,
		MaxAttempts:   5,
		BaseBackoff:   20 * time.Millisecond,
		MaxBackoff:    time.Second,
	}, logger)
	if err != nil {
		t.Fatalf("construct relay: %v", err)
	}
	if err := relay.Start(ctx); err != nil {
		t.Fatalf("start relay: %v", err)
	}
	t.Cleanup(func() { _ = relay.Close(context.Background()) })

	waitFor(t, ctx, "event publication and consumption", func() (bool, error) {
		var outboxStatus string
		var attempts int
		if err := pool.QueryRow(
			ctx,
			`SELECT status, attempt_count FROM platform.outbox_events WHERE id = $1`,
			committed.ID,
		).Scan(&outboxStatus, &attempts); err != nil {
			return false, err
		}
		var consumptions int
		if err := pool.QueryRow(
			ctx,
			`SELECT COUNT(*) FROM platform.event_consumptions
                 WHERE consumer_name = $1 AND event_id = $2`,
			eventconsumer.InfrastructureConsumer,
			committed.ID,
		).Scan(&consumptions); err != nil {
			return false, err
		}
		return outboxStatus == "published" && attempts == 2 && consumptions == 1, nil
	})

	payload, err := json.Marshal(committed)
	if err != nil {
		t.Fatalf("marshal duplicate event: %v", err)
	}
	jetStream, err := natsConnection.JetStream()
	if err != nil {
		t.Fatalf("get JetStream context: %v", err)
	}
	if _, err := jetStream.PublishMsg(
		&nats.Msg{Subject: eventconsumer.InfrastructureSubject, Data: payload},
		nats.MsgId(committed.ID+"-duplicate-proof"),
		nats.Context(ctx),
	); err != nil {
		t.Fatalf("publish duplicate delivery: %v", err)
	}
	waitFor(t, ctx, "duplicate delivery acknowledgement", func() (bool, error) {
		info, err := jetStream.ConsumerInfo(natsclient.EventsStream, eventconsumer.InfrastructureConsumer)
		if err != nil {
			return false, err
		}
		return info.AckFloor.Consumer >= 2, nil
	})

	var consumptionCount int
	if err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM platform.event_consumptions
         WHERE consumer_name = $1 AND event_id = $2`,
		eventconsumer.InfrastructureConsumer,
		committed.ID,
	).Scan(&consumptionCount); err != nil {
		t.Fatalf("count idempotent consumptions: %v", err)
	}
	if consumptionCount != 1 {
		t.Fatalf("expected one durable effect after duplicate delivery, got %d", consumptionCount)
	}

	var rolledBackCount int
	if err := pool.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM platform.outbox_events WHERE id = $1`,
		rolledBack.ID,
	).Scan(&rolledBackCount); err != nil {
		t.Fatalf("count rolled-back outbox event: %v", err)
	}
	if rolledBackCount != 0 {
		t.Fatalf("expected rolled-back event to be absent, got %d rows", rolledBackCount)
	}
}

func integrationEnvelope(t *testing.T, probe string) event.Envelope {
	t.Helper()
	identifiers := make([]string, 4)
	for index := range identifiers {
		value, err := identifier.NewUUIDv7()
		if err != nil {
			t.Fatalf("create event identifier: %v", err)
		}
		identifiers[index] = value
	}
	data, err := json.Marshal(map[string]string{"probe": probe})
	if err != nil {
		t.Fatalf("marshal proof data: %v", err)
	}
	return event.Envelope{
		ID:         identifiers[0],
		Type:       "infrastructure.probe",
		Version:    1,
		OccurredAt: time.Now().UTC(),
		Aggregate: event.Aggregate{
			Type: "infrastructure_probe",
			ID:   identifiers[1],
		},
		CorrelationID: identifiers[2],
		CausationID:   identifiers[3],
		Data:          data,
	}
}

func waitFor(
	t *testing.T,
	ctx context.Context,
	description string,
	condition func() (bool, error),
) {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		ready, err := condition()
		if err != nil {
			t.Fatalf("wait for %s: %v", description, err)
		}
		if ready {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for %s: %v", description, ctx.Err())
		case <-ticker.C:
		}
	}
}
