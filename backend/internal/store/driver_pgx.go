//go:build pgx

package store

// Registers the pgx database/sql driver. Built with `go build -tags pgx`
// (the Dockerfile does this); keeps the default build dependency-free.
import _ "github.com/jackc/pgx/v5/stdlib"

func init() { DriverName = "pgx" }
