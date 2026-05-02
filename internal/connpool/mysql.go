package connpool

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

// MySQLPing attempts to connect to a MySQL instance and ping it.
func MySQLPing(host string, port int, user, password string) error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/?timeout=5s", user, password, host, port)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open mysql: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping mysql: %w", err)
	}
	return nil
}

// MySQLGetTables connects to a MySQL database and returns the list of table names.
func MySQLGetTables(host string, port int, user, password, database string) ([]string, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?timeout=5s", user, password, host, port, database)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	defer db.Close()

	rows, err := db.Query("SHOW TABLES")
	if err != nil {
		return nil, fmt.Errorf("show tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}
