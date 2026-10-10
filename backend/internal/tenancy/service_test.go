package tenancy

import (
	"context"
	"errors"
	"testing"

	"github.com/lupppig/credar/backend/internal/platform/requestcontext"
)

const serviceActorID = "0199c8f0-4afb-71e3-b5d7-97bac4f520a8"

type authorizerStub struct {
	err                error
	platformPermission string
	tenantPermission   string
	tenantID           string
}

func (stub *authorizerStub) RequirePlatformPermission(_ context.Context, _ string, permission string) error {
	stub.platformPermission = permission
	return stub.err
}

func (stub *authorizerStub) RequireTenantPermission(
	_ context.Context,
	_ string,
	tenantID string,
	permission string,
) error {
	stub.tenantID = tenantID
	stub.tenantPermission = permission
	return stub.err
}

type storeStub struct {
	createTenantCalls     int
	createMembershipCalls int
	changeStatusCalls     int
}

func (stub *storeStub) CreateTenant(context.Context, CreateTenantCommand) (CreatedTenant, error) {
	stub.createTenantCalls++
	return CreatedTenant{}, nil
}

func (stub *storeStub) CreateMembership(context.Context, CreateMembershipCommand) (Membership, error) {
	stub.createMembershipCalls++
	return Membership{}, nil
}

func (stub *storeStub) ChangeMembershipStatus(context.Context, ChangeMembershipStatusCommand) (Membership, error) {
	stub.changeStatusCalls++
	return Membership{}, nil
}

func TestServiceChecksPlatformPermissionBeforeTenantCreation(t *testing.T) {
	denied := errors.New("denied")
	authorizer := &authorizerStub{err: denied}
	store := &storeStub{}
	service := NewService(authorizer, store)

	ctx := requestcontext.WithUserID(context.Background(), serviceActorID)
	_, err := service.CreateTenant(ctx, serviceActorID, CreateTenantCommand{})
	if !errors.Is(err, denied) {
		t.Fatalf("expected authorization error, got %v", err)
	}
	if authorizer.platformPermission != PermissionCreateTenant || store.createTenantCalls != 0 {
		t.Fatalf("tenant creation crossed authorization boundary")
	}
}

func TestServiceChecksTenantPermissionBeforeMembershipChanges(t *testing.T) {
	authorizer := &authorizerStub{}
	store := &storeStub{}
	service := NewService(authorizer, store)

	ctx := requestcontext.WithUserID(context.Background(), serviceActorID)
	_, err := service.CreateMembership(ctx, serviceActorID, CreateMembershipCommand{TenantID: "tenant"})
	if err != nil {
		t.Fatalf("create membership: %v", err)
	}
	if authorizer.tenantID != "tenant" || authorizer.tenantPermission != PermissionManageMembership || store.createMembershipCalls != 1 {
		t.Fatalf("membership creation did not use tenant permission")
	}

	_, err = service.ChangeMembershipStatus(ctx, serviceActorID, ChangeMembershipStatusCommand{TenantID: "tenant"})
	if err != nil {
		t.Fatalf("change membership status: %v", err)
	}
	if store.changeStatusCalls != 1 {
		t.Fatal("membership status store was not called")
	}
}

func TestServiceRejectsActorContextMismatch(t *testing.T) {
	authorizer := &authorizerStub{}
	store := &storeStub{}
	service := NewService(authorizer, store)

	_, err := service.CreateTenant(context.Background(), serviceActorID, CreateTenantCommand{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid actor context, got %v", err)
	}
	if store.createTenantCalls != 0 || authorizer.platformPermission != "" {
		t.Fatal("mismatched actor crossed the service boundary")
	}
}
