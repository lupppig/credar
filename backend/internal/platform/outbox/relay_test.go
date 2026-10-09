package outbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/lupppig/credar/backend/internal/platform/event"
	"github.com/lupppig/credar/backend/internal/platform/outbox"
)

func TestRelayPublishesAndMarksCommittedEvent(t *testing.T) {
	t.Parallel()

	repository := &fakeRelayRepository{
		records: []outbox.Record{{
			Envelope: relayEnvelope(),
			Subject:  "credar.events.infrastructure.probe.v1",
			Attempt:  1,
		}},
		publishedSignal: make(chan struct{}, 1),
		failureSignal:   make(chan struct{}, 1),
	}
	publisher := &fakePublisher{published: make(chan string, 1)}
	relay, err := outbox.NewRelay(repository, publisher, relayConfiguration(), discardLogger())
	if err != nil {
		t.Fatalf("construct relay: %v", err)
	}
	if err := relay.Start(context.Background()); err != nil {
		t.Fatalf("start relay: %v", err)
	}
	t.Cleanup(func() { _ = relay.Close(context.Background()) })

	select {
	case <-repository.publishedSignal:
	case <-time.After(time.Second):
		t.Fatal("relay did not complete publication")
	}

	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.published != relayEnvelope().ID {
		t.Fatalf("event was not marked published: %q", repository.published)
	}
}

func TestRelaySchedulesTransientPublishFailure(t *testing.T) {
	t.Parallel()

	repository := &fakeRelayRepository{
		records: []outbox.Record{{
			Envelope: relayEnvelope(),
			Subject:  "credar.events.infrastructure.probe.v1",
			Attempt:  2,
		}},
		publishedSignal: make(chan struct{}, 1),
		failureSignal:   make(chan struct{}, 1),
	}
	publisher := &fakePublisher{err: errors.New("NATS unavailable"), published: make(chan string, 1)}
	relay, err := outbox.NewRelay(repository, publisher, relayConfiguration(), discardLogger())
	if err != nil {
		t.Fatalf("construct relay: %v", err)
	}
	if err := relay.Start(context.Background()); err != nil {
		t.Fatalf("start relay: %v", err)
	}
	t.Cleanup(func() { _ = relay.Close(context.Background()) })

	select {
	case <-repository.failureSignal:
	case <-time.After(time.Second):
		t.Fatal("relay did not record publication failure")
	}

	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.failureClass != outbox.FailureTransient || repository.failureCode != "nats_publish_failed" {
		t.Fatalf("unexpected failure record: %s %s", repository.failureClass, repository.failureCode)
	}
}

func relayConfiguration() outbox.RelayConfig {
	return outbox.RelayConfig{
		PollInterval:  10 * time.Millisecond,
		LeaseDuration: time.Second,
		BatchSize:     10,
		MaxAttempts:   5,
		BaseBackoff:   time.Second,
		MaxBackoff:    time.Minute,
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func relayEnvelope() event.Envelope {
	return event.Envelope{
		ID:         "01991a4b-fa00-7000-8000-000000000001",
		Type:       "infrastructure.probe",
		Version:    1,
		OccurredAt: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC),
		Aggregate: event.Aggregate{
			Type: "infrastructure_probe",
			ID:   "01991a4b-fa00-7000-8000-000000000002",
		},
		CorrelationID: "01991a4b-fa00-7000-8000-000000000003",
		CausationID:   "01991a4b-fa00-7000-8000-000000000004",
		Data:          json.RawMessage(`{"probe":"committed"}`),
	}
}

type fakeRelayRepository struct {
	mu sync.Mutex

	records      []outbox.Record
	published    string
	failureClass outbox.FailureClass
	failureCode  string

	publishedSignal chan struct{}
	failureSignal   chan struct{}
}

func (repository *fakeRelayRepository) Claim(context.Context, string, int, time.Duration) ([]outbox.Record, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	records := repository.records
	repository.records = nil
	return records, nil
}

func (repository *fakeRelayRepository) MarkPublished(_ context.Context, eventID string, _ string) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.published = eventID
	repository.publishedSignal <- struct{}{}
	return nil
}

func (repository *fakeRelayRepository) MarkFailed(
	_ context.Context,
	_ string,
	_ string,
	class outbox.FailureClass,
	_ int,
	_ time.Time,
	errorCode string,
) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.failureClass = class
	repository.failureCode = errorCode
	repository.failureSignal <- struct{}{}
	return nil
}

type fakePublisher struct {
	err       error
	published chan string
}

func (publisher *fakePublisher) Publish(_ context.Context, _ string, eventID string, _ []byte) error {
	publisher.published <- eventID
	return publisher.err
}
