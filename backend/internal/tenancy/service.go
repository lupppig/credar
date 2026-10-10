package tenancy

import (
	"context"

	"github.com/lupppig/credar/backend/internal/platform/identifier"
	"github.com/lupppig/credar/backend/internal/platform/requestcontext"
)

const (
	PermissionCreateTenant     = "platform.tenants:create"
	PermissionManageMembership = "memberships:manage"
)

type Authorizer interface {
	RequirePlatformPermission(context.Context, string, string) error
	RequireTenantPermission(context.Context, string, string, string) error
}

type Store interface {
	CreateTenant(context.Context, CreateTenantCommand) (CreatedTenant, error)
	CreateMembership(context.Context, CreateMembershipCommand) (Membership, error)
	ChangeMembershipStatus(context.Context, ChangeMembershipStatusCommand) (Membership, error)
}

type Service struct {
	authorizer Authorizer
	store      Store
}

func NewService(authorizer Authorizer, store Store) *Service {
	return &Service{authorizer: authorizer, store: store}
}

func (service *Service) CreateTenant(
	ctx context.Context,
	actorUserID string,
	command CreateTenantCommand,
) (CreatedTenant, error) {
	if err := requireActorContext(ctx, actorUserID); err != nil {
		return CreatedTenant{}, err
	}
	if err := service.authorizer.RequirePlatformPermission(
		ctx,
		actorUserID,
		PermissionCreateTenant,
	); err != nil {
		return CreatedTenant{}, err
	}
	return service.store.CreateTenant(ctx, command)
}

func (service *Service) CreateMembership(
	ctx context.Context,
	actorUserID string,
	command CreateMembershipCommand,
) (Membership, error) {
	if err := requireActorContext(ctx, actorUserID); err != nil {
		return Membership{}, err
	}
	if err := service.authorizer.RequireTenantPermission(
		ctx,
		actorUserID,
		command.TenantID,
		PermissionManageMembership,
	); err != nil {
		return Membership{}, err
	}
	return service.store.CreateMembership(ctx, command)
}

func (service *Service) ChangeMembershipStatus(
	ctx context.Context,
	actorUserID string,
	command ChangeMembershipStatusCommand,
) (Membership, error) {
	if err := requireActorContext(ctx, actorUserID); err != nil {
		return Membership{}, err
	}
	if err := service.authorizer.RequireTenantPermission(
		ctx,
		actorUserID,
		command.TenantID,
		PermissionManageMembership,
	); err != nil {
		return Membership{}, err
	}
	return service.store.ChangeMembershipStatus(ctx, command)
}

func requireActorContext(ctx context.Context, actorUserID string) error {
	if !identifier.IsUUIDv7(actorUserID) || requestcontext.UserID(ctx) != actorUserID {
		return ErrInvalidInput
	}
	return nil
}
