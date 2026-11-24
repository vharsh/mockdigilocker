-- Mock DigiLocker schema (also applied automatically by the server on start).
-- Entities are JSON documents grouped by collection; see docs/architecture.md.
CREATE TABLE IF NOT EXISTS mdl_records (
    coll       TEXT        NOT NULL,
    id         TEXT        NOT NULL,
    seq        BIGSERIAL,
    data       JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (coll, id)
);
CREATE INDEX IF NOT EXISTS mdl_records_coll_seq ON mdl_records (coll, seq);
CREATE INDEX IF NOT EXISTS mdl_records_user ON mdl_records (coll, (data->>'user_id'));
CREATE INDEX IF NOT EXISTS mdl_records_client ON mdl_records (coll, (data->>'client_id'));
