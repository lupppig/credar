package tenancypostgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/lupppig/credar/backend/internal/identity"
	"github.com/lupppig/credar/backend/internal/platform/identifier"
)

func (repository *Repository) ResolveSessionContext(
	ctx context.Context,
	userID string,
	selectedTenantID string,
) ([]identity.MembershipSummary, *string, error) {
	if !identifier.IsUUIDv7(userID) {
		return nil, nil, identity.ErrTenantContextUnavailable
	}
	if selectedTenantID != "" && !identifier.IsUUIDv7(selectedTenantID) {
		return nil, nil, identity.ErrInvalidTenantContext
	}

	pool, err := repository.database.Pool()
	if err != nil {
		return nil, nil, errors.Join(identity.ErrTenantContextUnavailable, err)
	}
	rows, err := pool.Query(
		ctx,
		`SELECT memberships.id, memberships.status,
                tenants.id, tenants.name, tenants.status
         FROM tenancy.memberships AS memberships
         JOIN tenancy.tenants AS tenants ON tenants.id = memberships.tenant_id
         WHERE memberships.user_id = $1
           AND memberships.status = 'ACTIVE'
           AND tenants.status = 'ACTIVE'
         ORDER BY tenants.created_at, tenants.id, memberships.id`,
		userID,
	)
	if err != nil {
		return nil, nil, errors.Join(
			identity.ErrTenantContextUnavailable,
			fmt.Errorf("query active memberships: %w", err),
		)
	}
	defer rows.Close()

	memberships := make([]identity.MembershipSummary, 0)
	selectedFound := selectedTenantID == ""
	for rows.Next() {
		var membership identity.MembershipSummary
		if err := rows.Scan(
			&membership.ID,
			&membership.Status,
			&membership.Tenant.ID,
			&membership.Tenant.Name,
			&membership.Tenant.Status,
		); err != nil {
			return nil, nil, errors.Join(
				identity.ErrTenantContextUnavailable,
				fmt.Errorf("scan active membership: %w", err),
			)
		}
		membership.RoleNames = make([]string, 0)
		memberships = append(memberships, membership)
		if membership.Tenant.ID == selectedTenantID {
			selectedFound = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, errors.Join(
			identity.ErrTenantContextUnavailable,
			fmt.Errorf("iterate active memberships: %w", err),
		)
	}
	if !selectedFound {
		return nil, nil, identity.ErrMembershipInactive
	}
	if selectedTenantID == "" {
		return memberships, nil, nil
	}
	return memberships, &selectedTenantID, nil
}
