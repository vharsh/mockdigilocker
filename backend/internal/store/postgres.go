package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Schema is applied idempotently on start. It is also shipped as
// deploy/postgres/init.sql for teams that manage migrations externally.
const Schema = `
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
`

// Postgres stores records in a single JSONB table. The database/sql driver
// is registered by a build-tagged file (driver_pgx.go, tag "pgx").
type Postgres struct {
	db *sql.DB
}

// DriverName is set by the driver registration file when compiled in.
var DriverName = ""

func NewPostgres(dsn string) (*Postgres, error) {
	if DriverName == "" {
		return nil, errors.New("this binary was built without a PostgreSQL driver; rebuild with `-tags pgx` (the Docker image does this) or use STORE=memory")
	}
	db, err := sql.Open(DriverName, dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetConnMaxIdleTime(5 * time.Minute)
	var lastErr error
	for i := 0; i < 30; i++ { // wait for the database container to come up
		if lastErr = db.Ping(); lastErr == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if lastErr != nil {
		return nil, fmt.Errorf("postgres ping: %w", lastErr)
	}
	if _, err := db.Exec(Schema); err != nil {
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Postgres{db: db}, nil
}

func (p *Postgres) Put(coll, id string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = p.db.Exec(`INSERT INTO mdl_records (coll, id, data) VALUES ($1, $2, $3::jsonb)
		ON CONFLICT (coll, id) DO UPDATE SET data = EXCLUDED.data, updated_at = now()`, coll, id, string(raw))
	return err
}

func (p *Postgres) Get(coll, id string, v any) error {
	var raw []byte
	err := p.db.QueryRow(`SELECT data FROM mdl_records WHERE coll = $1 AND id = $2`, coll, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func (p *Postgres) Delete(coll, id string) error {
	_, err := p.db.Exec(`DELETE FROM mdl_records WHERE coll = $1 AND id = $2`, coll, id)
	return err
}

func (p *Postgres) Scan(coll string, fn func(id string, raw []byte) error) error {
	rows, err := p.db.Query(`SELECT id, data FROM mdl_records WHERE coll = $1 ORDER BY seq`, coll)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return err
		}
		if err := fn(id, raw); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (p *Postgres) Count(coll string) (int, error) {
	var n int
	err := p.db.QueryRow(`SELECT count(*) FROM mdl_records WHERE coll = $1`, coll).Scan(&n)
	return n, err
}

func (p *Postgres) Clear(coll string) error {
	_, err := p.db.Exec(`DELETE FROM mdl_records WHERE coll = $1`, coll)
	return err
}

func (p *Postgres) Trim(coll string, max int) error {
	_, err := p.db.Exec(`DELETE FROM mdl_records WHERE coll = $1 AND seq <= (
		SELECT seq FROM mdl_records WHERE coll = $1 ORDER BY seq DESC OFFSET $2 LIMIT 1)`, coll, max)
	return err
}

func (p *Postgres) Ping() error  { return p.db.Ping() }
func (p *Postgres) Close() error { return p.db.Close() }
