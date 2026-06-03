//go:build .lsc_no_postgres

package db

// Registers the Postgres driver with database/sql. Included by default;
// build with `-tags.lsc_no_postgres` to omit it (e.g. to drop the
// lib/pq dependency from a slim build).
import _ "github.com/lib/pq"
