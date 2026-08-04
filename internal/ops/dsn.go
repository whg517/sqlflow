package ops

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// postgresConn holds the connection details pg_dump needs as flags.
type postgresConn struct {
	host     string
	port     string
	user     string
	password string
	database string
	sslMode  string
	// schemas is the search_path from the DSN, if it names one. Empty means the
	// dump covers the whole database.
	schemas []string
}

// defaultPostgresPort is what libpq assumes when a DSN omits one.
const defaultPostgresPort = "5432"

// parsePostgresDSN splits a URL-style connection string into the parts pg_dump
// takes as separate flags.
//
// pg_dump accepts a connection URI directly, but only as a positional argument,
// which would put the password in argv where any user on the host can read it
// from ps. Splitting it here is what lets the password travel in the
// environment instead.
func parsePostgresDSN(dsn string) (postgresConn, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return postgresConn{}, fmt.Errorf("parse DSN: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return postgresConn{}, fmt.Errorf("unsupported DSN scheme %q: backups require a PostgreSQL connection string", u.Scheme)
	}

	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		// No port in the DSN is normal, not an error.
		host, port = u.Host, defaultPostgresPort
	}
	if host == "" {
		return postgresConn{}, fmt.Errorf("DSN has no host")
	}

	database := strings.TrimPrefix(u.Path, "/")
	if database == "" {
		return postgresConn{}, fmt.Errorf("DSN has no database name")
	}

	conn := postgresConn{
		host:     host,
		port:     port,
		user:     u.User.Username(),
		database: database,
		sslMode:  u.Query().Get("sslmode"),
	}
	conn.password, _ = u.User.Password()

	if searchPath := u.Query().Get("search_path"); searchPath != "" {
		for _, s := range strings.Split(searchPath, ",") {
			if s = strings.TrimSpace(s); s != "" {
				conn.schemas = append(conn.schemas, s)
			}
		}
	}

	return conn, nil
}
