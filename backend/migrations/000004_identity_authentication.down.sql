DROP TRIGGER IF EXISTS identity_audit_events_reject_delete ON identity.audit_events;
DROP TRIGGER IF EXISTS identity_audit_events_reject_update ON identity.audit_events;
DROP FUNCTION IF EXISTS identity.reject_audit_event_mutation();
DROP TABLE IF EXISTS identity.audit_events;
DROP TABLE IF EXISTS identity.users;
DROP SCHEMA IF EXISTS identity;
