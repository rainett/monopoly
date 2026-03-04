package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// wrapDBError wraps a database error with a descriptive action message
func wrapDBError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("failed to %s: %w", action, err)
}

// boolToInt converts a boolean to SQLite integer (0 or 1)
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// intToBool converts SQLite integer (0 or 1) to boolean
func intToBool(i int) bool {
	return i == 1
}

// InitDB initializes the database connection with proper configuration
func InitDB(dbPath string, maxOpenConnections, maxIdleConnections int) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db.SetMaxOpenConns(maxOpenConnections)
	db.SetMaxIdleConns(maxIdleConnections)

	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}
