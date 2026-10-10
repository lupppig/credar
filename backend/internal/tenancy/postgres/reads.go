package tenancypostgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/lupppig/credar/backend/internal/platform/identifier"
	"github.com/lupppig/credar/backend/internal/tenancy"
)

func (repository *Repository) ListTenants(ctx context.Context, limit int) ([]tenancy.Tenant, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	pool, err := repository.database.Pool()
	if err != nil {
		return nil, wrapPersistenceError(err)
	}
	rows, err := pool.Query(ctx, `SELECT id,name,status,created_at,updated_at FROM tenancy.tenants ORDER BY created_at,id LIMIT $1`, limit)
	if err != nil {
		return nil, wrapPersistenceError(err)
	}
	defer rows.Close()
	items := make([]tenancy.Tenant, 0)
	for rows.Next() {
		var item tenancy.Tenant
		if err := rows.Scan(&item.ID, &item.Name, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, wrapPersistenceError(err)
		}
		items = append(items, item)
	}
	return items, wrapPersistenceError(rows.Err())
}

func (repository *Repository) GetTenant(ctx context.Context, tenantID string) (tenancy.Tenant, error) {
	if !identifier.IsUUIDv7(tenantID) {
		return tenancy.Tenant{}, tenancy.ErrTenantNotFound
	}
	pool, err := repository.database.Pool()
	if err != nil {
		return tenancy.Tenant{}, wrapPersistenceError(err)
	}
	var item tenancy.Tenant
	err = pool.QueryRow(ctx, `SELECT id,name,status,created_at,updated_at FROM tenancy.tenants WHERE id=$1`, tenantID).Scan(&item.ID, &item.Name, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return tenancy.Tenant{}, tenancy.ErrTenantNotFound
	}
	return item, wrapPersistenceError(err)
}

func (repository *Repository) UpdateTenant(ctx context.Context, tenantID, name string) (tenancy.Tenant, error) {
	name = strings.TrimSpace(name)
	if !identifier.IsUUIDv7(tenantID) || !validName(name) {
		return tenancy.Tenant{}, tenancy.ErrInvalidInput
	}
	pool, err := repository.database.Pool()
	if err != nil {
		return tenancy.Tenant{}, wrapPersistenceError(err)
	}
	var item tenancy.Tenant
	err = pool.QueryRow(ctx, `UPDATE tenancy.tenants SET name=$2,updated_at=$3 WHERE id=$1 AND status='ACTIVE' RETURNING id,name,status,created_at,updated_at`, tenantID, name, time.Now().UTC()).Scan(&item.ID, &item.Name, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return tenancy.Tenant{}, tenancy.ErrTenantNotFound
	}
	return item, wrapPersistenceError(err)
}
