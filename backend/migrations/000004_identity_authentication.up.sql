CREATE SCHEMA IF NOT EXISTS identity;

CREATE TABLE identity.users (
    id UUID PRIMARY KEY,
    kratos_identity_id UUID NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT identity_users_email_normalized CHECK (email = LOWER(BTRIM(email))),
    CONSTRAINT identity_users_status_valid CHECK (status IN ('ACTIVE', 'DISABLED'))
);

CREATE INDEX identity_users_status_idx ON identity.users (status);

CREATE TABLE identity.audit_events (
    id UUID PRIMARY KEY,
    action TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id UUID,
    tenant_id UUID,
    target_type TEXT NOT NULL,
    target_id UUID NOT NULL,
    request_id UUID,
    correlation_id UUID NOT NULL,
    source TEXT NOT NULL,
    reason TEXT,
    changes JSONB NOT NULL DEFAULT '{}'::jsonb,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT identity_audit_actor_type_valid CHECK (actor_type IN ('USER', 'API_KEY', 'SYSTEM', 'WORKER')),
    CONSTRAINT identity_audit_source_valid CHECK (source IN ('HTTP', 'WORKER', 'SCHEDULER', 'PROVIDER_WEBHOOK', 'SYSTEM')),
    CONSTRAINT identity_audit_reason_length CHECK (reason IS NULL OR LENGTH(reason) BETWEEN 1 AND 2000),
    CONSTRAINT identity_audit_changes_object CHECK (jsonb_typeof(changes) = 'object')
);

CREATE INDEX identity_audit_events_target_idx
    ON identity.audit_events (target_type, target_id, occurred_at DESC);
CREATE INDEX identity_audit_events_correlation_idx
    ON identity.audit_events (correlation_id);
CREATE INDEX identity_audit_events_tenant_idx
    ON identity.audit_events (tenant_id, occurred_at DESC)
    WHERE tenant_id IS NOT NULL;

CREATE OR REPLACE FUNCTION identity.reject_audit_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'identity.audit_events is append-only';
END;
$$;

CREATE TRIGGER identity_audit_events_reject_update
BEFORE UPDATE ON identity.audit_events
FOR EACH ROW EXECUTE FUNCTION identity.reject_audit_event_mutation();

CREATE TRIGGER identity_audit_events_reject_delete
BEFORE DELETE ON identity.audit_events
FOR EACH ROW EXECUTE FUNCTION identity.reject_audit_event_mutation();
