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
		        tenants.id, tenants.name, tenants.status,
		        COALESCE(array_agg(roles.name ORDER BY roles.name)
		            FILTER (WHERE roles.id IS NOT NULL), ARRAY[]::text[])
         FROM tenancy.memberships AS memberships
         JOIN tenancy.tenants AS tenants ON tenants.id = memberships.tenant_id
		 LEFT JOIN identity.membership_role_assignments AS assignments
		   ON assignments.membership_id = memberships.id
		 LEFT JOIN identity.roles AS roles ON roles.id = assignments.role_id
         WHERE memberships.user_id = $1
           AND memberships.status = 'ACTIVE'
           AND tenants.status = 'ACTIVE'
		 GROUP BY memberships.id, memberships.status,
		          tenants.id, tenants.name, tenants.status, tenants.created_at
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
			&membership.RoleNames,
		); err != nil {
			return nil, nil, errors.Join(
				identity.ErrTenantContextUnavailable,
				fmt.Errorf("scan active membership: %w", err),
			)
		}
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
