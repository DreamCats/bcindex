package bcindex

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type VectorStore interface {
	EnsureCollection(ctx context.Context, name string, dims int) error
	UpsertPoints(ctx context.Context, collection string, points []VectorPoint) error
	DeletePointsByIDs(ctx context.Context, collection string, ids []string) error
	DeletePointsByRepo(ctx context.Context, collection string, repoID string) error
	DeletePointsByRepoAndPath(ctx context.Context, collection string, repoID, path string) error
	Close() error
}

type QdrantStore struct {
	client  *QdrantClient
	cleanup func()
}

func NewQdrantStore(cfg VectorConfig) (*QdrantStore, error) {
	ctx := context.Background()
	client, cleanup, err := NewQdrantClientFromConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &QdrantStore{client: client, cleanup: cleanup}, nil
}

func (s *QdrantStore) EnsureCollection(ctx context.Context, name string, dims int) error {
	return s.client.EnsureCollection(ctx, name, dims, "Cosine")
}

func (s *QdrantStore) UpsertPoints(ctx context.Context, collection string, points []VectorPoint) error {
	return s.client.UpsertPoints(ctx, collection, points)
}

func (s *QdrantStore) DeletePointsByIDs(ctx context.Context, collection string, ids []string) error {
	return s.client.DeletePointsByIDs(ctx, collection, ids)
}

func (s *QdrantStore) DeletePointsByRepo(ctx context.Context, collection string, repoID string) error {
	filter := qdrantMustFilter(qdrantMatchFilter("repo_id", repoID))
	return s.client.DeletePointsByFilter(ctx, collection, filter)
}

func (s *QdrantStore) DeletePointsByRepoAndPath(ctx context.Context, collection string, repoID, path string) error {
	filter := qdrantMustFilter(
		qdrantMatchFilter("repo_id", repoID),
		qdrantMatchFilter("path", path),
	)
	return s.client.DeletePointsByFilter(ctx, collection, filter)
}

func (s *QdrantStore) Close() error {
	if s.cleanup != nil {
		s.cleanup()
	}
	return nil
}

type LocalVectorStore struct {
	db *sql.DB
	mu sync.Mutex
}

func NewLocalVectorStore(path string) (*LocalVectorStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("qdrant_path is required for local vector store")
	}
	path = expandUserPath(path)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(path, "vectors.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open vector db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &LocalVectorStore{db: db}
	if err := store.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *LocalVectorStore) EnsureCollection(ctx context.Context, name string, dims int) error {
	return s.initSchema()
}

func (s *LocalVectorStore) UpsertPoints(ctx context.Context, collection string, points []VectorPoint) error {
	if len(points) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO vectors
		(id, repo_id, path, kind, name, title, line_start, line_end, hash, updated_at, vector)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, p := range points {
		vectorJSON, err := encodeVector(p.Vector)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		repoID := payloadString(p.Payload, "repo_id")
		path := payloadString(p.Payload, "path")
		kind := payloadString(p.Payload, "kind")
		name := payloadString(p.Payload, "name")
		title := payloadString(p.Payload, "title")
		lineStart := payloadInt64(p.Payload, "line_start")
		lineEnd := payloadInt64(p.Payload, "line_end")
		hash := payloadString(p.Payload, "hash")
		updatedAt := payloadInt64(p.Payload, "updated_at")
		if _, err := stmt.ExecContext(ctx,
			p.ID, repoID, path, kind, name, title, lineStart, lineEnd, hash, updatedAt, vectorJSON,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *LocalVectorStore) DeletePointsByIDs(ctx context.Context, collection string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	query, args := buildInClause("DELETE FROM vectors WHERE id IN (%s)", ids)
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *LocalVectorStore) DeletePointsByRepo(ctx context.Context, collection string, repoID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx, `DELETE FROM vectors WHERE repo_id = ?`, repoID)
	return err
}

func (s *LocalVectorStore) DeletePointsByRepoAndPath(ctx context.Context, collection string, repoID, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx, `DELETE FROM vectors WHERE repo_id = ? AND path = ?`, repoID, path)
	return err
}

func (s *LocalVectorStore) Close() error {
	return s.db.Close()
}

func (s *LocalVectorStore) initSchema() error {
	pragmas := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA synchronous=NORMAL;`,
		`PRAGMA busy_timeout=5000;`,
	}
	for _, stmt := range pragmas {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("init vector db: %w", err)
		}
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS vectors (
			id TEXT PRIMARY KEY,
			repo_id TEXT,
			path TEXT,
			kind TEXT,
			name TEXT,
			title TEXT,
			line_start INTEGER,
			line_end INTEGER,
			hash TEXT,
			updated_at INTEGER,
			vector TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_vectors_repo ON vectors (repo_id);`,
		`CREATE INDEX IF NOT EXISTS idx_vectors_repo_path ON vectors (repo_id, path);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("init vector db: %w", err)
		}
	}
	return nil
}

func encodeVector(vec []float32) (string, error) {
	data := make([]float64, len(vec))
	for i, val := range vec {
		data[i] = float64(val)
	}
	out, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	val, ok := payload[key]
	if !ok || val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func payloadInt64(payload map[string]any, key string) int64 {
	if payload == nil {
		return 0
	}
	val, ok := payload[key]
	if !ok || val == nil {
		return 0
	}
	switch v := val.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	default:
		return 0
	}
}

func buildInClause(template string, ids []string) (string, []any) {
	holders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		holders = append(holders, "?")
		args = append(args, id)
	}
	return fmt.Sprintf(template, strings.Join(holders, ",")), args
}
