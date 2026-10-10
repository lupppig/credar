CREATE TABLE identity.permissions (
    name TEXT PRIMARY KEY,
    scope TEXT NOT NULL,
    description TEXT NOT NULL,
    CONSTRAINT identity_permissions_name_valid CHECK (
        name ~ '^(platform\.)?[a-z][a-z0-9_]*:[a-z][a-z0-9_]*$'
    ),
    CONSTRAINT identity_permissions_scope_valid CHECK (scope IN ('PLATFORM', 'TENANT')),
    CONSTRAINT identity_permissions_description_valid CHECK (LENGTH(description) BETWEEN 1 AND 500)
);

CREATE TABLE identity.roles (
    id UUID PRIMARY KEY,
    tenant_id UUID REFERENCES tenancy.tenants (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    scope TEXT NOT NULL,
    kind TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT identity_roles_name_valid CHECK (
        name ~ '^[A-Za-z][A-Za-z0-9 _-]*$' AND LENGTH(name) BETWEEN 1 AND 100
    ),
    CONSTRAINT identity_roles_scope_valid CHECK (scope IN ('PLATFORM', 'TENANT')),
    CONSTRAINT identity_roles_kind_valid CHECK (kind IN ('SYSTEM', 'CUSTOM')),
    CONSTRAINT identity_roles_ownership_valid CHECK (
        (kind = 'SYSTEM' AND tenant_id IS NULL)
        OR (kind = 'CUSTOM' AND scope = 'TENANT' AND tenant_id IS NOT NULL)
    ),
    CONSTRAINT identity_roles_timestamps_valid CHECK (updated_at >= created_at)
);

CREATE UNIQUE INDEX identity_roles_system_name_idx
    ON identity.roles (scope, LOWER(name)) WHERE kind = 'SYSTEM';
CREATE UNIQUE INDEX identity_roles_tenant_name_idx
    ON identity.roles (tenant_id, LOWER(name)) WHERE kind = 'CUSTOM';
CREATE INDEX identity_roles_tenant_idx ON identity.roles (tenant_id, created_at, id);

CREATE TABLE identity.role_permissions (
    role_id UUID NOT NULL REFERENCES identity.roles (id) ON DELETE CASCADE,
    permission_name TEXT NOT NULL REFERENCES identity.permissions (name) ON DELETE RESTRICT,
    PRIMARY KEY (role_id, permission_name)
);

CREATE TABLE identity.platform_role_assignments (
    user_id UUID NOT NULL REFERENCES identity.users (id) ON DELETE RESTRICT,
    role_id UUID NOT NULL REFERENCES identity.roles (id) ON DELETE RESTRICT,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX identity_platform_role_assignments_role_idx
    ON identity.platform_role_assignments (role_id, user_id);

CREATE TABLE identity.membership_role_assignments (
    membership_id UUID NOT NULL REFERENCES tenancy.memberships (id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES identity.roles (id) ON DELETE RESTRICT,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (membership_id, role_id)
);

CREATE INDEX identity_membership_role_assignments_role_idx
    ON identity.membership_role_assignments (role_id, membership_id);

CREATE OR REPLACE FUNCTION identity.validate_role_permission_scope()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    role_scope TEXT;
    permission_scope TEXT;
BEGIN
    SELECT scope INTO role_scope FROM identity.roles WHERE id = NEW.role_id;
    SELECT scope INTO permission_scope FROM identity.permissions WHERE name = NEW.permission_name;
    IF role_scope IS NULL OR permission_scope IS NULL OR role_scope <> permission_scope THEN
        RAISE EXCEPTION 'role and permission scopes must match';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER identity_role_permissions_validate_scope
BEFORE INSERT OR UPDATE ON identity.role_permissions
FOR EACH ROW EXECUTE FUNCTION identity.validate_role_permission_scope();

CREATE OR REPLACE FUNCTION identity.validate_platform_role_assignment()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM identity.roles
        WHERE id = NEW.role_id AND scope = 'PLATFORM' AND kind = 'SYSTEM' AND tenant_id IS NULL
    ) THEN
        RAISE EXCEPTION 'platform assignments require a platform system role';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER identity_platform_role_assignments_validate
BEFORE INSERT OR UPDATE ON identity.platform_role_assignments
FOR EACH ROW EXECUTE FUNCTION identity.validate_platform_role_assignment();

CREATE OR REPLACE FUNCTION identity.validate_membership_role_assignment()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    membership_tenant_id UUID;
    role_tenant_id UUID;
    role_scope TEXT;
BEGIN
    SELECT tenant_id INTO membership_tenant_id
    FROM tenancy.memberships WHERE id = NEW.membership_id;
    SELECT tenant_id, scope INTO role_tenant_id, role_scope
    FROM identity.roles WHERE id = NEW.role_id;
    IF membership_tenant_id IS NULL OR role_scope <> 'TENANT'
       OR (role_tenant_id IS NOT NULL AND role_tenant_id <> membership_tenant_id) THEN
        RAISE EXCEPTION 'membership assignments require a role available to the same tenant';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER identity_membership_role_assignments_validate
BEFORE INSERT OR UPDATE ON identity.membership_role_assignments
FOR EACH ROW EXECUTE FUNCTION identity.validate_membership_role_assignment();

CREATE OR REPLACE FUNCTION identity.reject_system_role_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.kind = 'SYSTEM' THEN
        RAISE EXCEPTION 'system roles are immutable';
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$$;

CREATE TRIGGER identity_roles_reject_system_update
BEFORE UPDATE OR DELETE ON identity.roles
FOR EACH ROW EXECUTE FUNCTION identity.reject_system_role_mutation();

INSERT INTO identity.permissions (name, scope, description) VALUES
    ('platform.tenants:create', 'PLATFORM', 'Create a tenant and its initial owner'),
    ('platform.tenants:read', 'PLATFORM', 'Inspect tenants across the platform'),
    ('platform.memberships:manage', 'PLATFORM', 'Perform platform-authorized membership work'),
    ('tenants:read', 'TENANT', 'Read the current tenant'),
    ('tenants:manage', 'TENANT', 'Update allowed tenant settings'),
    ('memberships:read', 'TENANT', 'List and inspect tenant memberships'),
    ('memberships:manage', 'TENANT', 'Add, disable, remove, and reactivate members'),
    ('roles:read', 'TENANT', 'Read roles and permission assignments'),
    ('roles:manage', 'TENANT', 'Create and modify tenant custom roles'),
    ('audit:read', 'TENANT', 'Read the tenant audit history');

INSERT INTO identity.roles (id, name, scope, kind) VALUES
    ('01900000-0000-7000-8000-000000000001', 'PLATFORM_ADMIN', 'PLATFORM', 'SYSTEM'),
    ('01900000-0000-7000-8000-000000000002', 'TENANT_OWNER', 'TENANT', 'SYSTEM'),
    ('01900000-0000-7000-8000-000000000003', 'TENANT_ADMIN', 'TENANT', 'SYSTEM'),
    ('01900000-0000-7000-8000-000000000004', 'TENANT_AUDITOR', 'TENANT', 'SYSTEM'),
    ('01900000-0000-7000-8000-000000000005', 'TENANT_MEMBER', 'TENANT', 'SYSTEM');

INSERT INTO identity.role_permissions (role_id, permission_name)
SELECT '01900000-0000-7000-8000-000000000001', name
FROM identity.permissions WHERE scope = 'PLATFORM';

INSERT INTO identity.role_permissions (role_id, permission_name)
SELECT '01900000-0000-7000-8000-000000000002', name
FROM identity.permissions WHERE scope = 'TENANT';

INSERT INTO identity.role_permissions (role_id, permission_name)
SELECT '01900000-0000-7000-8000-000000000003', name
FROM identity.permissions
WHERE scope = 'TENANT' AND name <> 'tenants:manage';

INSERT INTO identity.role_permissions (role_id, permission_name) VALUES
    ('01900000-0000-7000-8000-000000000004', 'tenants:read'),
    ('01900000-0000-7000-8000-000000000004', 'memberships:read'),
    ('01900000-0000-7000-8000-000000000004', 'roles:read'),
    ('01900000-0000-7000-8000-000000000004', 'audit:read'),
    ('01900000-0000-7000-8000-000000000005', 'tenants:read');
