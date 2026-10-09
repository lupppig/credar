ALTER TABLE platform.outbox_events
    DROP CONSTRAINT outbox_subject_format;

UPDATE platform.outbox_events
SET subject = regexp_replace(subject, '^manoreck\.', 'credar.')
WHERE subject LIKE 'manoreck.%';

ALTER TABLE platform.outbox_events
    ADD CONSTRAINT outbox_subject_format
        CHECK (subject ~ '^credar\.events\.[a-z][a-z0-9_.]*\.v[1-9][0-9]*$');

UPDATE platform.event_consumptions
SET consumer_name = 'credar-infrastructure-proof-v1'
WHERE consumer_name = 'manoreck-infrastructure-proof-v1';

COMMENT ON SCHEMA platform IS
    'Internal runtime infrastructure owned by the Credar application';
