// Package outbox persists integration events with domain state and coordinates
// lease-based at-least-once publication outside database transactions.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lupppig/credar/backend/internal/platform/database"
	"github.com/lupppig/credar/backend/internal/platform/event"
	"github.com/lupppig/credar/backend/internal/platform/identifier"
)

// Record is one leased outbox event ready for publication.
type Record struct {
	Envelope event.Envelope
	Subject  string
	Attempt  int
}

// FailureClass controls whether a failed publication is retried.
type FailureClass string

const (
	// FailureTransient schedules bounded retry with backoff.
	FailureTransient FailureClass = "transient"
	// FailurePermanent moves the event directly to dead-letter state.
	FailurePermanent FailureClass = "permanent"
)

// Repository owns outbox persistence and publication state transitions.
type Repository struct {
	database *database.Database
}

// NewRepository constructs an outbox repository over the shared database.
func NewRepository(databaseConnection *database.Database) *Repository {
	return &Repository{database: databaseConnection}
}

// Insert records an integration event using the caller-owned transaction.
func (repository *Repository) Insert(
	ctx context.Context,
	transaction database.DBTX,
	envelope event.Envelope,
) error {
	if err := envelope.Validate(); err != nil {
		return fmt.Errorf("validate outbox event: %w", err)
	}
	subject, err := envelope.Subject()
	if err != nil {
		return fmt.Errorf("create outbox subject: %w", err)
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal outbox event: %w", err)
	}

	_, err = transaction.Exec(
		ctx,
		`INSERT INTO platform.outbox_events (
            id, tenant_id, event_type, event_version, aggregate_type,
            aggregate_id, subject, occurred_at, correlation_id, causation_id,
            payload
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		envelope.ID,
		envelope.TenantID,
		envelope.Type,
		envelope.Version,
		envelope.Aggregate.Type,
		envelope.Aggregate.ID,
		subject,
		envelope.OccurredAt.UTC(),
		envelope.CorrelationID,
		envelope.CausationID,
		payload,
	)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

// Claim leases pending or abandoned events in one short database transaction.
// Publication happens only after this transaction commits.
func (repository *Repository) Claim(
	ctx context.Context,
	workerID string,
	limit int,
	leaseDuration time.Duration,
) ([]Record, error) {
	if !identifier.IsUUIDv7(workerID) {
		return nil, fmt.Errorf("outbox worker ID must be a UUIDv7")
	}
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("outbox claim limit must be between 1 and 1000")
	}
	if leaseDuration <= 0 {
		return nil, fmt.Errorf("outbox lease duration must be positive")
	}

	var records []Record
	err := repository.database.InTransaction(ctx, pgx.TxOptions{}, func(transaction database.DBTX) error {
		rows, err := transaction.Query(
			ctx,
			`WITH candidates AS (
                SELECT id
                FROM platform.outbox_events
                WHERE (status = 'pending' AND next_attempt_at <= now())
                   OR (status = 'publishing' AND lease_expires_at <= now())
                ORDER BY created_at, id
                FOR UPDATE SKIP LOCKED
                LIMIT $1
            )
            UPDATE platform.outbox_events AS events
            SET status = 'publishing',
                attempt_count = events.attempt_count + 1,
                lease_owner = $2,
                lease_expires_at = now() + ($3 * interval '1 microsecond'),
                updated_at = now()
            FROM candidates
            WHERE events.id = candidates.id
            RETURNING events.payload, events.subject, events.attempt_count`,
			limit,
			workerID,
			leaseDuration.Microseconds(),
		)
		if err != nil {
			return fmt.Errorf("claim outbox events: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var payload []byte
			var record Record
			if err := rows.Scan(&payload, &record.Subject, &record.Attempt); err != nil {
				return fmt.Errorf("scan claimed outbox event: %w", err)
			}
			if err := json.Unmarshal(payload, &record.Envelope); err != nil {
				return fmt.Errorf("decode claimed outbox event: %w", err)
			}
			if err := record.Envelope.Validate(); err != nil {
				return fmt.Errorf("validate claimed outbox event: %w", err)
			}
			records = append(records, record)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate claimed outbox events: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}

// MarkPublished completes a lease after JetStream acknowledges publication.
func (repository *Repository) MarkPublished(ctx context.Context, eventID string, workerID string) error {
	pool, err := repository.database.Pool()
	if err != nil {
		return err
	}
	result, err := pool.Exec(
		ctx,
		`UPDATE platform.outbox_events
         SET status = 'published', published_at = now(), lease_owner = NULL,
             lease_expires_at = NULL, last_error_code = NULL, updated_at = now()
         WHERE id = $1 AND status = 'publishing' AND lease_owner = $2`,
		eventID,
		workerID,
	)
	if err != nil {
		return fmt.Errorf("mark outbox event published: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("outbox publication lease is no longer owned")
	}
	return nil
}

// MarkFailed releases a lease for retry or records a terminal dead letter.
func (repository *Repository) MarkFailed(
	ctx context.Context,
	eventID string,
	workerID string,
	class FailureClass,
	maxAttempts int,
	nextAttemptAt time.Time,
	errorCode string,
) error {
	if class != FailureTransient && class != FailurePermanent {
		return fmt.Errorf("unknown outbox failure class %q", class)
	}
	if maxAttempts < 1 {
		return fmt.Errorf("outbox maximum attempts must be positive")
	}
	if errorCode == "" {
		return fmt.Errorf("outbox failure code must not be empty")
	}

	pool, err := repository.database.Pool()
	if err != nil {
		return err
	}
	result, err := pool.Exec(
		ctx,
		`UPDATE platform.outbox_events
         SET status = CASE
                 WHEN $3 = 'permanent' OR attempt_count >= $4 THEN 'dead_letter'
                 ELSE 'pending'
             END,
             next_attempt_at = $5,
             lease_owner = NULL,
             lease_expires_at = NULL,
             dead_lettered_at = CASE
                 WHEN $3 = 'permanent' OR attempt_count >= $4 THEN now()
                 ELSE NULL
             END,
             last_error_code = $6,
             updated_at = now()
         WHERE id = $1 AND status = 'publishing' AND lease_owner = $2`,
		eventID,
		workerID,
		class,
		maxAttempts,
		nextAttemptAt.UTC(),
		errorCode,
	)
	if err != nil {
		return fmt.Errorf("record outbox publication failure: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("outbox failure lease is no longer owned")
	}
	return nil
}
