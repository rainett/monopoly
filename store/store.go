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

// InitDB initializes the database connection with proper configuration
// This is the only infrastructure function kept in the main store package
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

// Close closes the database connection
func Close(db *sql.DB) error {
	if db != nil {
		return db.Close()
	}
	return nil
}
