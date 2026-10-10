// Package authorizationtest provides reusable policy assertions for later
// tenant-owned capabilities without weakening the real authorization boundary.
package authorizationtest

import (
	"context"
	"errors"
	"testing"

	"github.com/lupppig/credar/backend/internal/authorization"
	"github.com/lupppig/credar/backend/internal/identity"
)

type TenantAuthorizer interface {
	RequireTenantPermission(context.Context, string, string, string) error
}

func RequireAllowed(t testing.TB, ctx context.Context, policy TenantAuthorizer, userID, tenantID, permission string) {
	t.Helper()
	if err := policy.RequireTenantPermission(ctx, userID, tenantID, permission); err != nil {
		t.Fatalf("expected %s to be allowed in tenant %s: %v", permission, tenantID, err)
	}
}

func RequirePermissionDenied(t testing.TB, ctx context.Context, policy TenantAuthorizer, userID, tenantID, permission string) {
	t.Helper()
	err := policy.RequireTenantPermission(ctx, userID, tenantID, permission)
	if !errors.Is(err, authorization.ErrPermissionDenied) {
		t.Fatalf("expected %s to be denied in tenant %s, got %v", permission, tenantID, err)
	}
}

func RequireTenantHidden(t testing.TB, ctx context.Context, policy TenantAuthorizer, userID, foreignTenantID, permission string) {
	t.Helper()
	err := policy.RequireTenantPermission(ctx, userID, foreignTenantID, permission)
	if !errors.Is(err, identity.ErrMembershipInactive) {
		t.Fatalf("expected foreign tenant %s to remain unavailable, got %v", foreignTenantID, err)
	}
}
