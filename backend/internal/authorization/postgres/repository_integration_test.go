//go:build integration

package authorizationpostgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lupppig/credar/backend/internal/authorization"
	authorizationpostgres "github.com/lupppig/credar/backend/internal/authorization/postgres"
	"github.com/lupppig/credar/backend/internal/identity"
	"github.com/lupppig/credar/backend/internal/platform/database"
	"github.com/lupppig/credar/backend/internal/platform/identifier"
	"github.com/lupppig/credar/backend/internal/platform/outbox"
	"github.com/lupppig/credar/backend/internal/platform/requestcontext"
	"github.com/lupppig/credar/backend/internal/tenancy"
	tenancypostgres "github.com/lupppig/credar/backend/internal/tenancy/postgres"
)

func TestAuthorizationIsolationAndEscalationBoundaries(t *testing.T) {
	db, ctx := startDatabase(t)
	truncateState(t, ctx, db)
	events := outbox.NewRepository(db)
	authorizer := authorizationpostgres.New(db, events)
	tenants := tenancypostgres.New(db, events)

	admin, err := authorizer.BootstrapPlatformAdmin(writeContext(t, ctx, ""), identity.ExternalIdentity{ID: uuid.NewString(), Email: "admin@example.com"})
	if err != nil {
		t.Fatalf("bootstrap administrator: %v", err)
	}
	if err := authorizer.RequirePlatformPermission(ctx, admin.ID, "platform.tenants:create"); err != nil {
		t.Fatalf("platform permission: %v", err)
	}
	_, err = authorizer.BootstrapPlatformAdmin(writeContext(t, ctx, ""), identity.ExternalIdentity{ID: uuid.NewString(), Email: "second@example.com"})
	if !errors.Is(err, authorization.ErrBootstrapAlreadyComplete) {
		t.Fatalf("expected one-time bootstrap denial, got %v", err)
	}

	created, err := tenants.CreateTenant(writeContext(t, ctx, admin.ID), tenancy.CreateTenantCommand{Name: "Alpha Finance", OwnerUserID: admin.ID, IdempotencyKey: "alpha-tenant-01"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := authorizer.RequireTenantPermission(ctx, admin.ID, created.Tenant.ID, "roles:manage"); err != nil {
		t.Fatalf("owner permission: %v", err)
	}

	_, err = authorizer.CreateRole(writeContext(t, ctx, admin.ID), admin.ID, created.Tenant.ID, authorization.RoleWrite{Name: "Escalated", PermissionNames: []string{"platform.tenants:create"}})
	if !errors.Is(err, authorization.ErrPermissionScopeEscalation) {
		t.Fatalf("expected scope escalation denial, got %v", err)
	}

	memberID := insertUser(t, ctx, db, "member@example.com")
	membership, err := tenants.CreateMembership(writeContext(t, ctx, admin.ID), tenancy.CreateMembershipCommand{TenantID: created.Tenant.ID, UserID: memberID, RoleIDs: []string{authorization.TenantMemberRoleID}})
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	if err := authorizer.RequireTenantPermission(ctx, memberID, created.Tenant.ID, "tenants:read"); err != nil {
		t.Fatalf("member read permission: %v", err)
	}
	if err := authorizer.RequireTenantPermission(ctx, memberID, created.Tenant.ID, "memberships:manage"); !errors.Is(err, authorization.ErrPermissionDenied) {
		t.Fatalf("expected denied management, got %v", err)
	}
	if err := authorizer.RequireTenantPermission(ctx, memberID, mustID(t), "tenants:read"); !errors.Is(err, identity.ErrMembershipInactive) {
		t.Fatalf("expected tenant substitution denial, got %v", err)
	}

	_, err = authorizer.ReplaceMembershipRoles(writeContext(t, ctx, admin.ID), admin.ID, created.Tenant.ID, membership.ID, []string{authorization.TenantOwnerRoleID}, "promote backup owner")
	if err != nil {
		t.Fatalf("promote owner: %v", err)
	}
	_, err = authorizer.ReplaceMembershipRoles(writeContext(t, ctx, admin.ID), admin.ID, created.Tenant.ID, created.InitialMembership.ID, []string{authorization.TenantMemberRoleID}, "transfer ownership")
	if err != nil {
		t.Fatalf("transfer ownership: %v", err)
	}
	_, err = tenants.ChangeMembershipStatus(writeContext(t, ctx, admin.ID), tenancy.ChangeMembershipStatusCommand{TenantID: created.Tenant.ID, MembershipID: membership.ID, Status: tenancy.MembershipStatusDisabled})
	if !errors.Is(err, authorization.ErrLastTenantOwnerRequired) {
		t.Fatalf("expected last-owner denial, got %v", err)
	}
}

func startDatabase(t *testing.T) (*database.Database, context.Context) {
	t.Helper()
	url := os.Getenv("CREDAR_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("CREDAR_TEST_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	db, err := database.New(database.Config{ConnectionString: url, MaxConnections: 8})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	return db, ctx
}
func truncateState(t *testing.T, ctx context.Context, db *database.Database) {
	t.Helper()
	pool, _ := db.Pool()
	_, err := pool.Exec(ctx, `DELETE FROM identity.membership_role_assignments;
		DELETE FROM identity.platform_role_assignments;
		DELETE FROM identity.roles WHERE kind = 'CUSTOM';
		DELETE FROM platform.idempotency_records;
		DELETE FROM tenancy.memberships;
		DELETE FROM tenancy.tenants;
		TRUNCATE identity.audit_events;
		DELETE FROM identity.users;
		DELETE FROM platform.outbox_events`)
	if err != nil {
		t.Fatal(err)
	}
}
func insertUser(t *testing.T, ctx context.Context, db *database.Database, email string) string {
	t.Helper()
	id := mustID(t)
	pool, _ := db.Pool()
	_, err := pool.Exec(ctx, `INSERT INTO identity.users(id,kratos_identity_id,email,status) VALUES($1,$2,$3,'ACTIVE')`, id, uuid.NewString(), email)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func mustID(t *testing.T) string {
	t.Helper()
	id, err := identifier.NewUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func writeContext(t *testing.T, parent context.Context, userID string) context.Context {
	t.Helper()
	requestID := mustID(t)
	correlationID := mustID(t)
	ctx := requestcontext.WithIdentifiers(parent, requestID, correlationID)
	if userID != "" {
		ctx = requestcontext.WithUserID(ctx, userID)
	}
	return ctx
}
