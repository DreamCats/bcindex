package store

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// RepositoryStore provides CRUD operations for repositories.
type RepositoryStore struct {
	db *DB
}

// NewRepositoryStore creates a new repository store.
func NewRepositoryStore(db *DB) *RepositoryStore {
	return &RepositoryStore{db: db}
}

// GetByRootPath retrieves a repository record by root path.
func (r *RepositoryStore) GetByRootPath(rootPath string) (*Repository, error) {
	query := `
		SELECT id, root_path, last_indexed_at, symbol_count, package_count,
			edge_count, has_embeddings, created_at, updated_at
		FROM repositories WHERE root_path = ?
	`

	row := r.db.sqlDB.QueryRow(query, rootPath)
	var repo Repository
	var lastIndexedValue any
	var hasEmbeddings int
	var createdAtValue any
	var updatedAtValue any

	err := row.Scan(
		&repo.ID, &repo.RootPath, &lastIndexedValue, &repo.SymbolCount,
		&repo.PackageCount, &repo.EdgeCount, &hasEmbeddings,
		&createdAtValue, &updatedAtValue,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	if ts, err := parseTimeValue(lastIndexedValue); err == nil && !ts.IsZero() {
		repo.LastIndexedAt = &ts
	} else if err != nil {
		return nil, fmt.Errorf("failed to parse last_indexed_at: %w", err)
	}

	createdAt, err := parseTimeValue(createdAtValue)
	if err != nil {
		return nil, fmt.Errorf("failed to parse created_at: %w", err)
	}
	repo.CreatedAt = createdAt

	updatedAt, err := parseTimeValue(updatedAtValue)
	if err != nil {
		return nil, fmt.Errorf("failed to parse updated_at: %w", err)
	}
	repo.UpdatedAt = updatedAt
	repo.HasEmbeddings = intToBool(hasEmbeddings)

	return &repo, nil
}

// Upsert inserts or updates a repository record.
func (r *RepositoryStore) Upsert(repo *Repository) error {
	if repo == nil {
		return fmt.Errorf("repository is nil")
	}
	if repo.RootPath == "" {
		return fmt.Errorf("repository root path is required")
	}

	if repo.ID == "" {
		repo.ID = repoID(repo.RootPath)
	}

	now := time.Now().UTC()
	if repo.CreatedAt.IsZero() {
		repo.CreatedAt = now
	}
	repo.UpdatedAt = now

	var lastIndexed any
	if repo.LastIndexedAt != nil && !repo.LastIndexedAt.IsZero() {
		lastIndexed = repo.LastIndexedAt.UTC().Format(time.RFC3339Nano)
	}

	query := `
		INSERT INTO repositories (
			id, root_path, last_indexed_at, symbol_count, package_count,
			edge_count, has_embeddings, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(root_path) DO UPDATE SET
			last_indexed_at = excluded.last_indexed_at,
			symbol_count = excluded.symbol_count,
			package_count = excluded.package_count,
			edge_count = excluded.edge_count,
			has_embeddings = excluded.has_embeddings,
			updated_at = excluded.updated_at
	`

	_, err := r.db.sqlDB.Exec(
		query,
		repo.ID, repo.RootPath, lastIndexed,
		repo.SymbolCount, repo.PackageCount, repo.EdgeCount,
		boolToInt(repo.HasEmbeddings), repo.CreatedAt.UTC().Format(time.RFC3339Nano),
		repo.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert repository: %w", err)
	}

	return nil
}

func repoID(rootPath string) string {
	hash := sha1.Sum([]byte(rootPath))
	return hex.EncodeToString(hash[:])
}
