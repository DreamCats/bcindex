package bcindex

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type VectorStore interface {
	EnsureCollection(ctx context.Context, name string, dims int) error
	UpsertPoints(ctx context.Context, collection string, points []VectorPoint) error
	DeletePointsByIDs(ctx context.Context, collection string, ids []string) error
	DeletePointsByRepo(ctx context.Context, collection string, repoID string) error
	DeletePointsByRepoAndPath(ctx context.Context, collection string, repoID, path string) error
	SearchSimilar(ctx context.Context, collection string, repoID string, vector []float32, topK int) ([]VectorSearchResult, error)
	Close() error
}

type VectorSearchResult struct {
	ID        string
	File      string
	Kind      string
	Name      string
	Title     string
	LineStart int
	LineEnd   int
	Score     float64
}

type VectorCandidate struct {
	File string
	Line int
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

func (s *QdrantStore) SearchSimilar(ctx context.Context, collection string, repoID string, vector []float32, topK int) ([]VectorSearchResult, error) {
	if topK <= 0 {
		topK = 10
	}
	filter := qdrantMustFilter(qdrantMatchFilter("repo_id", repoID))
	points, err := s.client.SearchPoints(ctx, collection, vector, topK, filter)
	if err != nil {
		return nil, err
	}
	results := make([]VectorSearchResult, 0, len(points))
	for _, p := range points {
		results = append(results, VectorSearchResult{
			ID:        p.ID,
			File:      payloadString(p.Payload, "path"),
			Kind:      payloadString(p.Payload, "kind"),
			Name:      payloadString(p.Payload, "name"),
			Title:     payloadString(p.Payload, "title"),
			LineStart: int(payloadInt64(p.Payload, "line_start")),
			LineEnd:   int(payloadInt64(p.Payload, "line_end")),
			Score:     p.Score,
		})
	}
	return results, nil
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

func (s *LocalVectorStore) SearchSimilar(ctx context.Context, collection string, repoID string, vector []float32, topK int) ([]VectorSearchResult, error) {
	if topK <= 0 {
		topK = 10
	}
	queryVec, queryNorm := toFloat64Vector(vector)
	if len(queryVec) == 0 || queryNorm == 0 {
		return nil, fmt.Errorf("vector query is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.QueryContext(ctx, `SELECT id, path, kind, name, title, line_start, line_end, vector FROM vectors WHERE repo_id = ?`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []VectorSearchResult
	for rows.Next() {
		var id, path, kind, name, title, vectorJSON string
		var lineStart, lineEnd int
		if err := rows.Scan(&id, &path, &kind, &name, &title, &lineStart, &lineEnd, &vectorJSON); err != nil {
			return nil, err
		}
		vec, err := decodeVector(vectorJSON)
		if err != nil {
			continue
		}
		score := cosineSimilarity(queryVec, vec, queryNorm)
		hits = append(hits, VectorSearchResult{
			ID:        id,
			File:      path,
			Kind:      kind,
			Name:      name,
			Title:     title,
			LineStart: lineStart,
			LineEnd:   lineEnd,
			Score:     score,
		})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > topK {
		hits = hits[:topK]
	}
	return hits, nil
}

func (s *LocalVectorStore) SearchSimilarCandidates(ctx context.Context, repoID string, vector []float32, candidates []VectorCandidate, topK int) ([]VectorSearchResult, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	if topK <= 0 {
		topK = 10
	}
	queryVec, queryNorm := toFloat64Vector(vector)
	if len(queryVec) == 0 || queryNorm == 0 {
		return nil, fmt.Errorf("vector query is empty")
	}
	uniq := make(map[string]VectorCandidate, len(candidates))
	for _, cand := range candidates {
		if cand.File == "" || cand.Line <= 0 {
			continue
		}
		key := fmt.Sprintf("%s:%d", cand.File, cand.Line)
		uniq[key] = cand
	}
	if len(uniq) == 0 {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stmt, err := s.db.PrepareContext(ctx, `SELECT id, path, kind, name, title, line_start, line_end, vector FROM vectors
		WHERE repo_id = ? AND path = ? AND line_start <= ? AND line_end >= ?`)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	best := make(map[string]VectorSearchResult, len(uniq))
	for _, cand := range uniq {
		rows, err := stmt.QueryContext(ctx, repoID, cand.File, cand.Line, cand.Line)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, path, kind, name, title, vectorJSON string
			var lineStart, lineEnd int
			if err := rows.Scan(&id, &path, &kind, &name, &title, &lineStart, &lineEnd, &vectorJSON); err != nil {
				_ = rows.Close()
				return nil, err
			}
			vec, err := decodeVector(vectorJSON)
			if err != nil {
				continue
			}
			score := cosineSimilarity(queryVec, vec, queryNorm)
			key := fmt.Sprintf("%s:%d", cand.File, cand.Line)
			if existing, ok := best[key]; !ok || score > existing.Score {
				best[key] = VectorSearchResult{
					ID:        id,
					File:      path,
					Kind:      kind,
					Name:      name,
					Title:     title,
					LineStart: cand.Line,
					LineEnd:   lineEnd,
					Score:     score,
				}
			}
		}
		_ = rows.Close()
	}
	results := make([]VectorSearchResult, 0, len(best))
	for _, hit := range best {
		results = append(results, hit)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
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

func decodeVector(raw string) ([]float64, error) {
	var vec []float64
	if err := json.Unmarshal([]byte(raw), &vec); err != nil {
		return nil, err
	}
	return vec, nil
}

func toFloat64Vector(vec []float32) ([]float64, float64) {
	out := make([]float64, len(vec))
	var sum float64
	for i, val := range vec {
		v := float64(val)
		out[i] = v
		sum += v * v
	}
	return out, math.Sqrt(sum)
}

func cosineSimilarity(query []float64, vec []float64, queryNorm float64) float64 {
	if len(query) == 0 || len(vec) == 0 || queryNorm == 0 {
		return 0
	}
	if len(query) != len(vec) {
		return 0
	}
	var dot float64
	var norm float64
	for i, val := range vec {
		dot += query[i] * val
		norm += val * val
	}
	if norm == 0 {
		return 0
	}
	return dot / (queryNorm * math.Sqrt(norm))
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
