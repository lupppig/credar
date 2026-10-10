package tenancypostgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/lupppig/credar/backend/internal/authorization"
	"github.com/lupppig/credar/backend/internal/platform/database"
	"github.com/lupppig/credar/backend/internal/platform/event"
	"github.com/lupppig/credar/backend/internal/tenancy"
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

func wrapPersistenceError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, tenancy.ErrInvalidInput) ||
		errors.Is(err, tenancy.ErrIdempotencyKeyReused) ||
		errors.Is(err, tenancy.ErrTenantNotFound) ||
		errors.Is(err, tenancy.ErrUserNotFound) ||
		errors.Is(err, tenancy.ErrMembershipNotFound) ||
		errors.Is(err, tenancy.ErrMembershipExists) ||
		errors.Is(err, tenancy.ErrInvalidTransition) ||
		errors.Is(err, authorization.ErrPermissionScopeEscalation) ||
		errors.Is(err, authorization.ErrLastTenantOwnerRequired) {
		return err
	}
	return errors.Join(tenancy.ErrUnavailable, err)
}

func lockKey(ctx context.Context, transaction database.DBTX, key string) error {
	if _, err := transaction.Exec(
		ctx,
		"SELECT pg_advisory_xact_lock(hashtextextended($1, 0))",
		key,
	); err != nil {
		return fmt.Errorf("lock tenancy operation: %w", err)
	}
	return nil
}

func requireActiveTenantAndUser(
	ctx context.Context,
	transaction database.DBTX,
	tenantID string,
	userID string,
) error {
	var tenantStatus string
	if err := transaction.QueryRow(
		ctx,
		`SELECT status FROM tenancy.tenants WHERE id = $1`,
		tenantID,
	).Scan(&tenantStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tenancy.ErrTenantNotFound
		}
		return fmt.Errorf("query tenant: %w", err)
	}
	if tenantStatus != string(tenancy.TenantStatusActive) {
		return tenancy.ErrTenantNotFound
	}
	return requireActiveUser(ctx, transaction, userID)
}

func requireActiveUser(
	ctx context.Context,
	transaction database.DBTX,
	userID string,
) error {
	var userStatus string
	if err := transaction.QueryRow(
		ctx,
		`SELECT status FROM identity.users WHERE id = $1`,
		userID,
	).Scan(&userStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tenancy.ErrUserNotFound
		}
		return fmt.Errorf("query application user: %w", err)
	}
	if userStatus != "ACTIVE" {
		return tenancy.ErrUserNotFound
	}
	return nil
}
