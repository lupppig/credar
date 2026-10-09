// Package natsclient owns the NATS connection and shared JetStream topology.
package natsclient

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	// EventsStream is the durable stream for internal and dead-letter subjects.
	EventsStream  = "CREDAR_EVENTS"
	eventsSubject = "credar.events.>"
	deadSubject   = "credar.dead.>"
)

// Client manages one reconnecting NATS connection and JetStream context.
type Client struct {
	url string

	mu         sync.RWMutex
	connection *nats.Conn
	jetStream  nats.JetStreamContext
}

// New constructs the NATS dependency without opening a connection.
func New(url string) (*Client, error) {
	if url == "" {
		return nil, fmt.Errorf("NATS URL must not be empty")
	}
	return &Client{url: url}, nil
}

// Name identifies the dependency in readiness output.
func (*Client) Name() string {
	return "nats"
}

// Start connects and establishes the version-independent stream topology.
func (client *Client) Start(context.Context) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.connection != nil {
		return fmt.Errorf("NATS client has already started")
	}

	connection, err := nats.Connect(
		client.url,
		nats.Name("credar-api"),
		nats.Timeout(5*time.Second),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(time.Second),
	)
	if err != nil {
		return fmt.Errorf("connect to NATS: %w", err)
	}
	jetStream, err := connection.JetStream(nats.PublishAsyncMaxPending(256))
	if err != nil {
		connection.Close()
		return fmt.Errorf("create JetStream context: %w", err)
	}
	if err := ensureEventsStream(jetStream); err != nil {
		connection.Close()
		return err
	}

	client.connection = connection
	client.jetStream = jetStream
	return nil
}

// Check verifies the connection and required stream without changing message
// state.
func (client *Client) Check(ctx context.Context) error {
	client.mu.RLock()
	connection := client.connection
	jetStream := client.jetStream
	client.mu.RUnlock()
	if connection == nil || jetStream == nil {
		return fmt.Errorf("NATS client is not started")
	}
	if !connection.IsConnected() {
		return fmt.Errorf("NATS connection is not ready")
	}
	if err := connection.FlushWithContext(ctx); err != nil {
		return fmt.Errorf("flush NATS connection: %w", err)
	}
	if _, err := jetStream.StreamInfo(EventsStream, nats.Context(ctx)); err != nil {
		return fmt.Errorf("inspect JetStream events stream: %w", err)
	}
	return nil
}

// Close drains subscriptions and published messages before closing.
func (client *Client) Close(ctx context.Context) error {
	client.mu.Lock()
	connection := client.connection
	client.connection = nil
	client.jetStream = nil
	client.mu.Unlock()
	if connection == nil {
		return nil
	}

	drained := make(chan error, 1)
	go func() {
		drained <- connection.Drain()
	}()
	select {
	case err := <-drained:
		if err != nil && !errors.Is(err, nats.ErrConnectionClosed) {
			return fmt.Errorf("drain NATS connection: %w", err)
		}
		return nil
	case <-ctx.Done():
		connection.Close()
		return fmt.Errorf("drain NATS connection: %w", ctx.Err())
	}
}

// JetStream returns the started context for relay and consumer construction.
func (client *Client) JetStream() (nats.JetStreamContext, error) {
	client.mu.RLock()
	defer client.mu.RUnlock()
	if client.jetStream == nil {
		return nil, fmt.Errorf("JetStream client is not started")
	}
	return client.jetStream, nil
}

// Publish sends one event with stable JetStream deduplication identity.
func (client *Client) Publish(
	ctx context.Context,
	subject string,
	eventID string,
	payload []byte,
) error {
	jetStream, err := client.JetStream()
	if err != nil {
		return err
	}
	message := &nats.Msg{Subject: subject, Data: payload}
	if _, err := jetStream.PublishMsg(message, nats.MsgId(eventID), nats.Context(ctx)); err != nil {
		return fmt.Errorf("publish JetStream event: %w", err)
	}
	return nil
}

func ensureEventsStream(jetStream nats.JetStreamContext) error {
	configuration := &nats.StreamConfig{
		Name:       EventsStream,
		Subjects:   []string{eventsSubject, deadSubject},
		Retention:  nats.LimitsPolicy,
		Storage:    nats.FileStorage,
		Discard:    nats.DiscardOld,
		MaxAge:     30 * 24 * time.Hour,
		Duplicates: 10 * time.Minute,
		Replicas:   1,
	}
	if _, err := jetStream.StreamInfo(EventsStream); err != nil {
		if !errors.Is(err, nats.ErrStreamNotFound) {
			return fmt.Errorf("inspect JetStream events stream: %w", err)
		}
		if _, err := jetStream.AddStream(configuration); err != nil {
			return fmt.Errorf("create JetStream events stream: %w", err)
		}
		return nil
	}
	if _, err := jetStream.UpdateStream(configuration); err != nil {
		return fmt.Errorf("update JetStream events stream: %w", err)
	}
	return nil
}
