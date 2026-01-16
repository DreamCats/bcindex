package store

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

const (
	// CurrentSchemaVersion is the version of the database schema
	CurrentSchemaVersion = 1
)

// DB manages the SQLite database connection and schema migrations
type DB struct {
	sqlDB *sql.DB
	path  string
}

// Open opens or creates a database at the given path
func Open(path string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Open database with optimizations
	sqlDB, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode=WAL&_pragma=synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &DB{
		sqlDB: sqlDB,
		path:  path,
	}

	// Run migrations
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return db, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.sqlDB.Close()
}

// SQLDB returns the underlying *sql.DB for direct queries
func (db *DB) SQLDB() *sql.DB {
	return db.sqlDB
}

// migrate runs schema migrations
func (db *DB) migrate() error {
	// Get current schema version
	version, err := db.getSchemaVersion()
	if err != nil {
		return fmt.Errorf("failed to get schema version: %w", err)
	}

	// If already at current version, we're done
	if version >= CurrentSchemaVersion {
		return nil
	}

	// Begin transaction
	tx, err := db.sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// For now, we just apply the full schema for fresh installs
	// In the future, we'd have incremental migrations
	if version == 0 {
		// Fresh install - apply full schema
		schema, err := schemaFS.ReadFile("schema.sql")
		if err != nil {
			return fmt.Errorf("failed to read schema: %w", err)
		}

		if _, err := tx.Exec(string(schema)); err != nil {
			return fmt.Errorf("failed to apply schema: %w", err)
		}

		// Set schema version
		if _, err := tx.Exec(
			"INSERT INTO schema_version (version, applied_at) VALUES (?, ?)",
			CurrentSchemaVersion,
			time.Now().UTC().Format(time.RFC3339),
		); err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
	} else {
		// TODO: Implement incremental migrations
		// For now, we'd need to handle version 0 -> 1, etc.
		return fmt.Errorf("incremental migrations not yet implemented (current version: %d, target: %d)", version, CurrentSchemaVersion)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}

	return nil
}

// getSchemaVersion returns the current schema version
func (db *DB) getSchemaVersion() (int, error) {
	var version int

	// Check if schema_version table exists
	var exists int
	if err := db.sqlDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_version'",
	).Scan(&exists); err != nil {
		return 0, fmt.Errorf("failed to check schema_version table: %w", err)
	}

	if exists == 0 {
		return 0, nil
	}

	// Get current version
	if err := db.sqlDB.QueryRow("SELECT version FROM schema_version ORDER BY version DESC LIMIT 1").Scan(&version); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get schema version: %w", err)
	}

	return version, nil
}

// Clear removes all data from the database (useful for re-indexing)
func (db *DB) Clear() error {
	tx, err := db.sqlDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Clear all tables (preserve schema)
	tables := []string{
		"indexing_jobs",
		"repositories",
		"embeddings",
		"packages_fts",
		"packages",
		"edges",
		"symbols_fts",
		"symbols",
		"schema_version",
	}

	for _, table := range tables {
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			// Table might not exist, that's OK
			continue
		}
	}

	// Reset schema version
	if _, err := tx.Exec(
		"INSERT INTO schema_version (version, applied_at) VALUES (?, ?)",
		CurrentSchemaVersion,
		time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("failed to reset schema version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit clear: %w", err)
	}

	return nil
}

// BeginTx starts a new transaction
func (db *DB) BeginTx() (*sql.Tx, error) {
	return db.sqlDB.Begin()
}

// Stats returns database statistics
func (db *DB) Stats() (*DBStats, error) {
	stats := &DBStats{}

	// Get symbol count
	if err := db.sqlDB.QueryRow("SELECT COUNT(*) FROM symbols").Scan(&stats.SymbolCount); err != nil {
		return nil, fmt.Errorf("failed to get symbol count: %w", err)
	}

	// Get package count
	if err := db.sqlDB.QueryRow("SELECT COUNT(*) FROM packages").Scan(&stats.PackageCount); err != nil {
		return nil, fmt.Errorf("failed to get package count: %w", err)
	}

	// Get edge count
	if err := db.sqlDB.QueryRow("SELECT COUNT(*) FROM edges").Scan(&stats.EdgeCount); err != nil {
		return nil, fmt.Errorf("failed to get edge count: %w", err)
	}

	// Get repository count
	if err := db.sqlDB.QueryRow("SELECT COUNT(*) FROM repositories").Scan(&stats.RepositoryCount); err != nil {
		return nil, fmt.Errorf("failed to get repository count: %w", err)
	}

	// Get database size
	if info, err := os.Stat(db.path); err == nil {
		stats.SizeBytes = info.Size()
	}

	return stats, nil
}

// DBStats represents database statistics
type DBStats struct {
	SymbolCount     int64
	PackageCount    int64
	EdgeCount       int64
	RepositoryCount int64
	SizeBytes       int64
}
