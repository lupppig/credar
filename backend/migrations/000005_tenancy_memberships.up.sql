CREATE SCHEMA IF NOT EXISTS tenancy;

CREATE TABLE tenancy.tenants (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT tenancy_tenants_name_valid CHECK (
        name = BTRIM(name) AND LENGTH(name) BETWEEN 1 AND 160
    ),
    CONSTRAINT tenancy_tenants_status_valid CHECK (status IN ('ACTIVE', 'DISABLED')),
    CONSTRAINT tenancy_tenants_timestamps_valid CHECK (updated_at >= created_at)
);

CREATE INDEX tenancy_tenants_created_idx
    ON tenancy.tenants (created_at, id);

CREATE TABLE tenancy.memberships (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenancy.tenants (id) ON DELETE RESTRICT,
    user_id UUID NOT NULL REFERENCES identity.users (id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    disabled_at TIMESTAMPTZ,
    removed_at TIMESTAMPTZ,
    CONSTRAINT tenancy_memberships_status_valid CHECK (
        status IN ('ACTIVE', 'DISABLED', 'REMOVED')
    ),
    CONSTRAINT tenancy_memberships_timestamps_valid CHECK (
        updated_at >= created_at
        AND (disabled_at IS NULL OR disabled_at >= created_at)
        AND (removed_at IS NULL OR removed_at >= created_at)
    ),
    CONSTRAINT tenancy_memberships_status_timestamps_match CHECK (
        (status = 'ACTIVE' AND disabled_at IS NULL AND removed_at IS NULL)
        OR (status = 'DISABLED' AND disabled_at IS NOT NULL AND removed_at IS NULL)
        OR (status = 'REMOVED' AND removed_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX tenancy_memberships_current_user_idx
    ON tenancy.memberships (tenant_id, user_id)
    WHERE status <> 'REMOVED';
CREATE INDEX tenancy_memberships_tenant_page_idx
    ON tenancy.memberships (tenant_id, created_at, id);
CREATE INDEX tenancy_memberships_user_active_idx
    ON tenancy.memberships (user_id, tenant_id)
    WHERE status = 'ACTIVE';

CREATE TABLE platform.idempotency_records (
    actor_user_id UUID NOT NULL REFERENCES identity.users (id) ON DELETE RESTRICT,
    scope_tenant_id UUID REFERENCES tenancy.tenants (id) ON DELETE RESTRICT,
    operation TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    response_status INTEGER NOT NULL,
    response_body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (actor_user_id, operation, idempotency_key),
    CONSTRAINT platform_idempotency_operation_valid CHECK (
        operation ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$'
    ),
    CONSTRAINT platform_idempotency_key_valid CHECK (
        LENGTH(idempotency_key) BETWEEN 8 AND 255
        AND idempotency_key ~ '^[!-~]+$'
    ),
    CONSTRAINT platform_idempotency_hash_valid CHECK (
        request_hash ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT platform_idempotency_status_valid CHECK (
        response_status BETWEEN 200 AND 299
    ),
    CONSTRAINT platform_idempotency_response_object CHECK (
        jsonb_typeof(response_body) = 'object'
    ),
    CONSTRAINT platform_idempotency_expiry_valid CHECK (expires_at > created_at)
);

CREATE INDEX platform_idempotency_expiry_idx
    ON platform.idempotency_records (expires_at);
