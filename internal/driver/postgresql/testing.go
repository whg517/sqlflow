package postgresql

import (
	"database/sql"

	"github.com/whg517/sqlflow/internal/driver"
)

// NewWithDB wraps an already-open *sql.DB as a connected PostgreSQL driver.
//
// It exists so tests can drive the real query and metadata code paths against a
// mock database instead of a live server. Production code must go through
// Connect: this constructor bypasses configuration validation and pool setup.
func NewWithDB(db *sql.DB) driver.Driver {
	return &PostgreSQLDriver{db: db, schema: "public"}
}
