package identitypostgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/lupppig/credar/backend/internal/identity"
	"github.com/lupppig/credar/backend/internal/platform/database"
	"github.com/lupppig/credar/backend/internal/platform/event"
	"github.com/lupppig/credar/backend/internal/platform/identifier"
	"github.com/lupppig/credar/backend/internal/platform/requestcontext"
)

type EventInserter interface {
	Insert(context.Context, database.DBTX, event.Envelope) error
}

type Repository struct {
	database *database.Database
	events   EventInserter
}

func New(databaseConnection *database.Database, events EventInserter) *Repository {
	return &Repository{database: databaseConnection, events: events}
}

func (repository *Repository) Resolve(
	ctx context.Context,
	externalIdentity identity.ExternalIdentity,
) (identity.User, error) {
	kratosIdentityID := strings.ToLower(strings.TrimSpace(externalIdentity.ID))
	email := strings.ToLower(strings.TrimSpace(externalIdentity.Email))
	if _, err := uuid.Parse(kratosIdentityID); err != nil || email == "" {
		return identity.User{}, fmt.Errorf("%w: invalid external identity", identity.ErrAuthenticationUnavailable)
	}

	requestID := requestcontext.RequestID(ctx)
	correlationID := requestcontext.CorrelationID(ctx)
	if !identifier.IsUUIDv7(requestID) || !identifier.IsUUIDv7(correlationID) {
		return identity.User{}, fmt.Errorf("%w: request identifiers are missing", identity.ErrAuthenticationUnavailable)
	}

	var resolved identity.User
	err := repository.database.InTransaction(ctx, pgx.TxOptions{}, func(transaction database.DBTX) error {
		if err := lockMappingKeys(ctx, transaction, kratosIdentityID, email); err != nil {
			return err
		}

		matches, err := findMatches(ctx, transaction, kratosIdentityID, email)
		if err != nil {
			return err
		}
		if len(matches) > 1 {
			return identity.ErrIdentityMappingConflict
		}
		if len(matches) == 1 {
			if matches[0].KratosIdentityID != kratosIdentityID {
				return identity.ErrIdentityMappingConflict
			}
			resolved = matches[0]
			return nil
		}

		resolved, err = repository.createUser(
			ctx,
			transaction,
			kratosIdentityID,
			email,
			requestID,
			correlationID,
		)
		return err
	})
	if errors.Is(err, identity.ErrIdentityMappingConflict) {
		return identity.User{}, identity.ErrIdentityMappingConflict
	}
	if err != nil {
		return identity.User{}, errors.Join(identity.ErrAuthenticationUnavailable, err)
	}
	return resolved, nil
}

func lockMappingKeys(
	ctx context.Context,
	transaction database.DBTX,
	kratosIdentityID string,
	email string,
) error {
	keys := []string{"identity:" + kratosIdentityID, "email:" + email}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := transaction.Exec(
			ctx,
			"SELECT pg_advisory_xact_lock(hashtextextended($1, 0))",
			key,
		); err != nil {
			return fmt.Errorf("lock identity mapping: %w", err)
		}
	}
	return nil
}

func findMatches(
	ctx context.Context,
	transaction database.DBTX,
	kratosIdentityID string,
	email string,
) ([]identity.User, error) {
	rows, err := transaction.Query(
		ctx,
		`SELECT id, kratos_identity_id, email, status, created_at, updated_at
         FROM identity.users
         WHERE kratos_identity_id = $1 OR email = $2
         ORDER BY id`,
		kratosIdentityID,
		email,
	)
	if err != nil {
		return nil, fmt.Errorf("query identity mappings: %w", err)
	}
	defer rows.Close()

	var matches []identity.User
	for rows.Next() {
		var user identity.User
		if err := rows.Scan(
			&user.ID,
			&user.KratosIdentityID,
			&user.Email,
			&user.Status,
			&user.CreatedAt,
			&user.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan identity mapping: %w", err)
		}
		matches = append(matches, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate identity mappings: %w", err)
	}
	return matches, nil
}

func (repository *Repository) createUser(
	ctx context.Context,
	transaction database.DBTX,
	kratosIdentityID string,
	email string,
	requestID string,
	correlationID string,
) (identity.User, error) {
	userID, err := identifier.NewUUIDv7()
	if err != nil {
		return identity.User{}, fmt.Errorf("create user ID: %w", err)
	}
	auditID, err := identifier.NewUUIDv7()
	if err != nil {
		return identity.User{}, fmt.Errorf("create audit event ID: %w", err)
	}
	eventID, err := identifier.NewUUIDv7()
	if err != nil {
		return identity.User{}, fmt.Errorf("create integration event ID: %w", err)
	}

	now := time.Now().UTC()
	user := identity.User{
		ID:               userID,
		KratosIdentityID: kratosIdentityID,
		Email:            email,
		Status:           identity.StatusActive,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if _, err := transaction.Exec(
		ctx,
		`INSERT INTO identity.users (
            id, kratos_identity_id, email, status, created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $5)`,
		user.ID,
		user.KratosIdentityID,
		user.Email,
		user.Status,
		now,
	); err != nil {
		return identity.User{}, fmt.Errorf("insert identity mapping: %w", err)
	}

	if _, err := transaction.Exec(
		ctx,
		`INSERT INTO identity.audit_events (
            id, action, actor_type, actor_id, target_type, target_id,
            request_id, correlation_id, source, changes, occurred_at
        ) VALUES ($1, 'identity.user_linked', 'USER', $2, 'user', $2,
            $3, $4, 'HTTP', '{}'::jsonb, $5)`,
		auditID,
		user.ID,
		requestID,
		correlationID,
		now,
	); err != nil {
		return identity.User{}, fmt.Errorf("insert identity audit event: %w", err)
	}

	eventData, err := json.Marshal(struct {
		UserID string `json:"user_id"`
	}{UserID: user.ID})
	if err != nil {
		return identity.User{}, fmt.Errorf("marshal identity event: %w", err)
	}
	if err := repository.events.Insert(ctx, transaction, event.Envelope{
		ID:         eventID,
		Type:       "identity.user_linked",
		Version:    1,
		OccurredAt: now,
		TenantID:   nil,
		Aggregate: event.Aggregate{
			Type: "user",
			ID:   user.ID,
		},
		CorrelationID: correlationID,
		CausationID:   requestID,
		Data:          eventData,
	}); err != nil {
		return identity.User{}, fmt.Errorf("insert identity integration event: %w", err)
	}

	return user, nil
}
