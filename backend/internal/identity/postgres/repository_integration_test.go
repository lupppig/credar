//go:build integration

package identitypostgres_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lupppig/credar/backend/internal/identity"
	identitypostgres "github.com/lupppig/credar/backend/internal/identity/postgres"
	"github.com/lupppig/credar/backend/internal/platform/database"
	"github.com/lupppig/credar/backend/internal/platform/event"
	"github.com/lupppig/credar/backend/internal/platform/identifier"
	"github.com/lupppig/credar/backend/internal/platform/outbox"
	"github.com/lupppig/credar/backend/internal/platform/requestcontext"
)

func TestIdentityMappingIsAtomicAndConflictSafe(t *testing.T) {
	databaseConnection, ctx := startDatabase(t)
	pool, err := databaseConnection.Pool()
	if err != nil {
		t.Fatalf("get pool: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`DELETE FROM identity.membership_role_assignments;
		 DELETE FROM identity.platform_role_assignments;
		 DELETE FROM identity.roles WHERE kind = 'CUSTOM';
		 DELETE FROM platform.idempotency_records;
		 DELETE FROM tenancy.memberships;
		 DELETE FROM tenancy.tenants;
		 TRUNCATE identity.audit_events;
		 DELETE FROM identity.users;
		 DELETE FROM platform.outbox_events`,
	); err != nil {
		t.Fatalf("truncate identity state: %v", err)
	}

	repository := identitypostgres.New(databaseConnection, outbox.NewRepository(databaseConnection))
	externalIdentity := identity.ExternalIdentity{ID: uuid.NewString(), Email: "person@example.com"}

	first, err := repository.Resolve(newRequestContext(t, ctx), externalIdentity)
	if err != nil {
		t.Fatalf("resolve first session: %v", err)
	}
	second, err := repository.Resolve(newRequestContext(t, ctx), externalIdentity)
	if err != nil {
		t.Fatalf("resolve repeated session: %v", err)
	}
	if first.ID != second.ID || !identifier.IsUUIDv7(first.ID) {
		t.Fatalf("mapping was not stable: first=%+v second=%+v", first, second)
	}
	assertCounts(t, ctx, databaseConnection, 1, 1, 1)

	_, err = repository.Resolve(newRequestContext(t, ctx), identity.ExternalIdentity{
		ID:    uuid.NewString(),
		Email: externalIdentity.Email,
	})
	if !errors.Is(err, identity.ErrIdentityMappingConflict) {
		t.Fatalf("expected email mapping conflict, got %v", err)
	}
	assertCounts(t, ctx, databaseConnection, 1, 1, 1)

	concurrentIdentity := identity.ExternalIdentity{ID: uuid.NewString(), Email: "concurrent@example.com"}
	const workers = 8
	results := make(chan identity.User, workers)
	errorsChannel := make(chan error, workers)
	var waitGroup sync.WaitGroup
	for range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			user, resolveErr := repository.Resolve(newRequestContext(t, ctx), concurrentIdentity)
			results <- user
			errorsChannel <- resolveErr
		}()
	}
	waitGroup.Wait()
	close(results)
	close(errorsChannel)

	for resolveErr := range errorsChannel {
		if resolveErr != nil {
			t.Fatalf("resolve concurrent session: %v", resolveErr)
		}
	}
	var concurrentUserID string
	for user := range results {
		if concurrentUserID == "" {
			concurrentUserID = user.ID
		}
		if user.ID != concurrentUserID {
			t.Fatalf("concurrent mappings returned different users: %q and %q", concurrentUserID, user.ID)
		}
	}
	assertCounts(t, ctx, databaseConnection, 2, 2, 2)
}

func TestIdentityMappingRollsBackWhenOutboxInsertFails(t *testing.T) {
	databaseConnection, ctx := startDatabase(t)
	pool, err := databaseConnection.Pool()
	if err != nil {
		t.Fatalf("get pool: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`DELETE FROM identity.membership_role_assignments;
		 DELETE FROM identity.platform_role_assignments;
		 DELETE FROM identity.roles WHERE kind = 'CUSTOM';
		 DELETE FROM platform.idempotency_records;
		 DELETE FROM tenancy.memberships;
		 DELETE FROM tenancy.tenants;
		 TRUNCATE identity.audit_events;
		 DELETE FROM identity.users;
		 DELETE FROM platform.outbox_events`,
	); err != nil {
		t.Fatalf("truncate identity state: %v", err)
	}

	repository := identitypostgres.New(databaseConnection, failingEventInserter{})
	_, err = repository.Resolve(newRequestContext(t, ctx), identity.ExternalIdentity{
		ID:    uuid.NewString(),
		Email: "rollback@example.com",
	})
	if !errors.Is(err, identity.ErrAuthenticationUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	assertCounts(t, ctx, databaseConnection, 0, 0, 0)
}

type failingEventInserter struct{}

func (failingEventInserter) Insert(context.Context, database.DBTX, event.Envelope) error {
	return errors.New("forced outbox failure")
}

func startDatabase(t *testing.T) (*database.Database, context.Context) {
	t.Helper()
	connectionString := os.Getenv("CREDAR_TEST_DATABASE_URL")
	if connectionString == "" {
		t.Skip("CREDAR_TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	databaseConnection, err := database.New(database.Config{
		ConnectionString: connectionString,
		MaxConnections:   12,
		MinConnections:   0,
	})
	if err != nil {
		t.Fatalf("construct database: %v", err)
	}
	if err := databaseConnection.Start(ctx); err != nil {
		t.Fatalf("start database: %v", err)
	}
	t.Cleanup(func() {
		if err := databaseConnection.Close(context.Background()); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	return databaseConnection, ctx
}

func newRequestContext(t *testing.T, parent context.Context) context.Context {
	t.Helper()
	requestID, err := identifier.NewUUIDv7()
	if err != nil {
		t.Fatalf("create request ID: %v", err)
	}
	correlationID, err := identifier.NewUUIDv7()
	if err != nil {
		t.Fatalf("create correlation ID: %v", err)
	}
	return requestcontext.WithIdentifiers(parent, requestID, correlationID)
}

func assertCounts(
	t *testing.T,
	ctx context.Context,
	databaseConnection *database.Database,
	wantUsers int,
	wantAudits int,
	wantEvents int,
) {
	t.Helper()
	pool, err := databaseConnection.Pool()
	if err != nil {
		t.Fatalf("get pool: %v", err)
	}
	var users, audits, events int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM identity.users`).Scan(&users); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM identity.audit_events`).Scan(&audits); err != nil {
		t.Fatalf("count audits: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM platform.outbox_events WHERE event_type = 'identity.user_linked'`).Scan(&events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if users != wantUsers || audits != wantAudits || events != wantEvents {
		t.Fatalf(
			"unexpected counts: users=%d audits=%d events=%d, want %d/%d/%d",
			users,
			audits,
			events,
			wantUsers,
			wantAudits,
			wantEvents,
		)
	}
}
