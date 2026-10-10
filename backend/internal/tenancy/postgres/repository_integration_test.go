//go:build integration

package tenancypostgres_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lupppig/credar/backend/internal/identity"
	"github.com/lupppig/credar/backend/internal/platform/database"
	"github.com/lupppig/credar/backend/internal/platform/event"
	"github.com/lupppig/credar/backend/internal/platform/identifier"
	"github.com/lupppig/credar/backend/internal/platform/outbox"
	"github.com/lupppig/credar/backend/internal/platform/requestcontext"
	"github.com/lupppig/credar/backend/internal/tenancy"
	tenancypostgres "github.com/lupppig/credar/backend/internal/tenancy/postgres"
)

func TestTenantAndMembershipLifecycle(t *testing.T) {
	databaseConnection, ctx := startDatabase(t)
	truncateTenancyState(t, ctx, databaseConnection)
	actorUserID := insertUser(t, ctx, databaseConnection, "actor@example.com")
	memberUserID := insertUser(t, ctx, databaseConnection, "member@example.com")
	repository := tenancypostgres.New(databaseConnection, outbox.NewRepository(databaseConnection))

	create := tenancy.CreateTenantCommand{
		Name:           "Northstar Finance",
		OwnerUserID:    actorUserID,
		IdempotencyKey: "tenant-create-001",
	}
	first, err := repository.CreateTenant(newWriteContext(t, ctx, actorUserID), create)
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	replayed, err := repository.CreateTenant(newWriteContext(t, ctx, actorUserID), create)
	if err != nil {
		t.Fatalf("replay tenant creation: %v", err)
	}
	if replayed.Tenant.ID != first.Tenant.ID || replayed.InitialMembership.ID != first.InitialMembership.ID {
		t.Fatalf("idempotent replay returned a different outcome: first=%+v replay=%+v", first, replayed)
	}

	conflicting := create
	conflicting.Name = "Different Tenant"
	_, err = repository.CreateTenant(newWriteContext(t, ctx, actorUserID), conflicting)
	if !errors.Is(err, tenancy.ErrIdempotencyKeyReused) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}

	concurrent := tenancy.CreateTenantCommand{
		Name:           "Concurrent Tenant",
		OwnerUserID:    actorUserID,
		IdempotencyKey: "tenant-create-concurrent",
	}
	const workers = 8
	contexts := make([]context.Context, workers)
	for index := range contexts {
		contexts[index] = newWriteContext(t, ctx, actorUserID)
	}
	results := make(chan tenancy.CreatedTenant, workers)
	errorsChannel := make(chan error, workers)
	var waitGroup sync.WaitGroup
	for index := range workers {
		waitGroup.Add(1)
		go func(worker int) {
			defer waitGroup.Done()
			result, createErr := repository.CreateTenant(contexts[worker], concurrent)
			results <- result
			errorsChannel <- createErr
		}(index)
	}
	waitGroup.Wait()
	close(results)
	close(errorsChannel)
	for createErr := range errorsChannel {
		if createErr != nil {
			t.Fatalf("create concurrent tenant: %v", createErr)
		}
	}
	var concurrentTenantID string
	for result := range results {
		if concurrentTenantID == "" {
			concurrentTenantID = result.Tenant.ID
		}
		if result.Tenant.ID != concurrentTenantID {
			t.Fatalf("concurrent creation returned multiple tenants: %q and %q", concurrentTenantID, result.Tenant.ID)
		}
	}
	assertCounts(t, ctx, databaseConnection, 2, 2, 4, 4, 2)

	memberships, activeTenantID, err := repository.ResolveSessionContext(ctx, actorUserID, first.Tenant.ID)
	if err != nil {
		t.Fatalf("resolve selected tenant: %v", err)
	}
	if len(memberships) != 2 || activeTenantID == nil || *activeTenantID != first.Tenant.ID {
		t.Fatalf("unexpected owner session context: memberships=%+v active=%v", memberships, activeTenantID)
	}
	_, _, err = repository.ResolveSessionContext(ctx, actorUserID, "not-a-uuid")
	if !errors.Is(err, identity.ErrInvalidTenantContext) {
		t.Fatalf("expected invalid tenant context, got %v", err)
	}
	unknownTenantID := mustUUIDv7(t)
	_, _, err = repository.ResolveSessionContext(ctx, actorUserID, unknownTenantID)
	if !errors.Is(err, identity.ErrMembershipInactive) {
		t.Fatalf("expected inactive membership, got %v", err)
	}

	membership, err := repository.CreateMembership(
		newWriteContext(t, ctx, actorUserID),
		tenancy.CreateMembershipCommand{TenantID: first.Tenant.ID, UserID: memberUserID},
	)
	if err != nil {
		t.Fatalf("create membership: %v", err)
	}
	_, err = repository.CreateMembership(
		newWriteContext(t, ctx, actorUserID),
		tenancy.CreateMembershipCommand{TenantID: first.Tenant.ID, UserID: memberUserID},
	)
	if !errors.Is(err, tenancy.ErrMembershipExists) {
		t.Fatalf("expected duplicate membership conflict, got %v", err)
	}

	memberContext, _, err := repository.ResolveSessionContext(ctx, memberUserID, "")
	if err != nil || len(memberContext) != 1 {
		t.Fatalf("expected active member context, memberships=%+v err=%v", memberContext, err)
	}

	disabled, err := repository.ChangeMembershipStatus(
		newWriteContext(t, ctx, actorUserID),
		tenancy.ChangeMembershipStatusCommand{
			TenantID:     first.Tenant.ID,
			MembershipID: membership.ID,
			Status:       tenancy.MembershipStatusDisabled,
			Reason:       "access paused",
		},
	)
	if err != nil || disabled.DisabledAt == nil {
		t.Fatalf("disable membership: membership=%+v err=%v", disabled, err)
	}
	_, _, err = repository.ResolveSessionContext(ctx, memberUserID, first.Tenant.ID)
	if !errors.Is(err, identity.ErrMembershipInactive) {
		t.Fatalf("expected disabled membership denial, got %v", err)
	}

	reactivated, err := repository.ChangeMembershipStatus(
		newWriteContext(t, ctx, actorUserID),
		tenancy.ChangeMembershipStatusCommand{
			TenantID:     first.Tenant.ID,
			MembershipID: membership.ID,
			Status:       tenancy.MembershipStatusActive,
		},
	)
	if err != nil || reactivated.DisabledAt != nil {
		t.Fatalf("reactivate membership: membership=%+v err=%v", reactivated, err)
	}

	removed, err := repository.ChangeMembershipStatus(
		newWriteContext(t, ctx, actorUserID),
		tenancy.ChangeMembershipStatusCommand{
			TenantID:     first.Tenant.ID,
			MembershipID: membership.ID,
			Status:       tenancy.MembershipStatusRemoved,
			Reason:       "user left organization",
		},
	)
	if err != nil || removed.RemovedAt == nil {
		t.Fatalf("remove membership: membership=%+v err=%v", removed, err)
	}
	_, err = repository.ChangeMembershipStatus(
		newWriteContext(t, ctx, actorUserID),
		tenancy.ChangeMembershipStatusCommand{
			TenantID:     first.Tenant.ID,
			MembershipID: membership.ID,
			Status:       tenancy.MembershipStatusActive,
		},
	)
	if !errors.Is(err, tenancy.ErrInvalidTransition) {
		t.Fatalf("expected removed membership to remain terminal, got %v", err)
	}

	rejoined, err := repository.CreateMembership(
		newWriteContext(t, ctx, actorUserID),
		tenancy.CreateMembershipCommand{TenantID: first.Tenant.ID, UserID: memberUserID},
	)
	if err != nil {
		t.Fatalf("rejoin membership: %v", err)
	}
	if rejoined.ID == membership.ID {
		t.Fatal("rejoining must create a new membership identity")
	}

	_, err = repository.ChangeMembershipStatus(
		newWriteContext(t, ctx, actorUserID),
		tenancy.ChangeMembershipStatusCommand{
			TenantID:     concurrentTenantID,
			MembershipID: rejoined.ID,
			Status:       tenancy.MembershipStatusDisabled,
		},
	)
	if !errors.Is(err, tenancy.ErrMembershipNotFound) {
		t.Fatalf("expected cross-tenant membership lookup to be hidden, got %v", err)
	}
	assertCounts(t, ctx, databaseConnection, 2, 4, 9, 9, 2)
}

func TestTenantCreationRollsBackWhenEventInsertFails(t *testing.T) {
	databaseConnection, ctx := startDatabase(t)
	truncateTenancyState(t, ctx, databaseConnection)
	actorUserID := insertUser(t, ctx, databaseConnection, "rollback-actor@example.com")
	repository := tenancypostgres.New(databaseConnection, failingEventInserter{})

	_, err := repository.CreateTenant(
		newWriteContext(t, ctx, actorUserID),
		tenancy.CreateTenantCommand{
			Name:           "Rollback Tenant",
			OwnerUserID:    actorUserID,
			IdempotencyKey: "tenant-rollback-001",
		},
	)
	if !errors.Is(err, tenancy.ErrUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	assertCounts(t, ctx, databaseConnection, 0, 0, 0, 0, 0)
}

type failingEventInserter struct{}

func (failingEventInserter) Insert(context.Context, database.DBTX, event.Envelope) error {
	return errors.New("forced event insert failure")
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

func truncateTenancyState(t *testing.T, ctx context.Context, databaseConnection *database.Database) {
	t.Helper()
	pool, err := databaseConnection.Pool()
	if err != nil {
		t.Fatalf("get pool: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`TRUNCATE platform.idempotency_records, tenancy.memberships,
            tenancy.tenants, identity.audit_events, identity.users,
            platform.outbox_events`,
	); err != nil {
		t.Fatalf("truncate tenancy state: %v", err)
	}
}

func insertUser(
	t *testing.T,
	ctx context.Context,
	databaseConnection *database.Database,
	email string,
) string {
	t.Helper()
	userID := mustUUIDv7(t)
	pool, err := databaseConnection.Pool()
	if err != nil {
		t.Fatalf("get pool: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO identity.users (id, kratos_identity_id, email, status)
         VALUES ($1, $2, $3, 'ACTIVE')`,
		userID,
		uuid.NewString(),
		email,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return userID
}

func newWriteContext(t *testing.T, parent context.Context, actorUserID string) context.Context {
	t.Helper()
	requestID := mustUUIDv7(t)
	correlationID := mustUUIDv7(t)
	ctx := requestcontext.WithIdentifiers(parent, requestID, correlationID)
	return requestcontext.WithUserID(ctx, actorUserID)
}

func mustUUIDv7(t *testing.T) string {
	t.Helper()
	value, err := identifier.NewUUIDv7()
	if err != nil {
		t.Fatalf("create UUIDv7: %v", err)
	}
	return value
}

func assertCounts(
	t *testing.T,
	ctx context.Context,
	databaseConnection *database.Database,
	wantTenants int,
	wantMemberships int,
	wantAudits int,
	wantEvents int,
	wantIdempotency int,
) {
	t.Helper()
	pool, err := databaseConnection.Pool()
	if err != nil {
		t.Fatalf("get pool: %v", err)
	}
	queries := []struct {
		name string
		sql  string
		want int
	}{
		{name: "tenants", sql: `SELECT COUNT(*) FROM tenancy.tenants`, want: wantTenants},
		{name: "memberships", sql: `SELECT COUNT(*) FROM tenancy.memberships`, want: wantMemberships},
		{name: "audits", sql: `SELECT COUNT(*) FROM identity.audit_events`, want: wantAudits},
		{name: "events", sql: `SELECT COUNT(*) FROM platform.outbox_events`, want: wantEvents},
		{name: "idempotency", sql: `SELECT COUNT(*) FROM platform.idempotency_records`, want: wantIdempotency},
	}
	for _, query := range queries {
		var actual int
		if err := pool.QueryRow(ctx, query.sql).Scan(&actual); err != nil {
			t.Fatalf("count %s: %v", query.name, err)
		}
		if actual != query.want {
			t.Fatalf("unexpected %s count: got %d, want %d", query.name, actual, query.want)
		}
	}
}
