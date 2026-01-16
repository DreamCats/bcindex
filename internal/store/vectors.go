package store

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/DreamCats/bcindex/internal/embedding"
)

// VectorStore provides vector storage and similarity search operations
type VectorStore struct {
	db *DB
}

// NewVectorStore creates a new vector store
func NewVectorStore(db *DB) *VectorStore {
	return &VectorStore{db: db}
}

// ScoredResult represents a search result with similarity score
type ScoredResult struct {
	SymbolID string
	Score    float32
	Distance float32
	Symbol   *Symbol
}

// Insert inserts or updates a vector for a symbol
func (v *VectorStore) Insert(symbolID string, vector []float32, model string) error {
	if len(vector) == 0 {
		return fmt.Errorf("cannot insert empty vector")
	}

	// Convert vector to binary blob
	blob, err := vectorToBlob(vector)
	if err != nil {
		return fmt.Errorf("failed to convert vector to blob: %w", err)
	}

	query := `
		INSERT OR REPLACE INTO embeddings (symbol_id, vector, dimension, model, created_at)
		VALUES (?, ?, ?, ?, ?)
	`

	_, err = v.db.sqlDB.Exec(query, symbolID, blob, len(vector), model, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("failed to insert vector: %w", err)
	}

	return nil
}

// InsertBatch inserts multiple vectors in a transaction
func (v *VectorStore) InsertBatch(symbolIDs []string, vectors [][]float32, model string) error {
	if len(symbolIDs) != len(vectors) {
		return fmt.Errorf("symbolIDs and vectors length mismatch")
	}

	if len(symbolIDs) == 0 {
		return nil
	}

	tx, err := v.db.BeginTx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT OR REPLACE INTO embeddings (symbol_id, vector, dimension, model, created_at)
		VALUES (?, ?, ?, ?, ?)
	`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)

	for i, vector := range vectors {
		if len(vector) == 0 {
			continue
		}

		blob, err := vectorToBlob(vector)
		if err != nil {
			return fmt.Errorf("failed to convert vector %d to blob: %w", i, err)
		}

		if _, err := stmt.Exec(symbolIDs[i], blob, len(vector), model, now); err != nil {
			return fmt.Errorf("failed to insert vector %d: %w", i, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// Get retrieves a vector for a symbol
func (v *VectorStore) Get(symbolID string) ([]float32, error) {
	var blob []byte
	var dimension int

	query := "SELECT vector, dimension FROM embeddings WHERE symbol_id = ?"
	err := v.db.sqlDB.QueryRow(query, symbolID).Scan(&blob, &dimension)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("vector not found for symbol: %s", symbolID)
		}
		return nil, fmt.Errorf("failed to get vector: %w", err)
	}

	vector, err := blobToVector(blob)
	if err != nil {
		return nil, fmt.Errorf("failed to convert blob to vector: %w", err)
	}

	if len(vector) != dimension {
		return nil, fmt.Errorf("vector dimension mismatch: expected %d, got %d", dimension, len(vector))
	}

	return vector, nil
}

// Search performs similarity search using cosine similarity
func (v *VectorStore) Search(queryVector []float32, topK int, symbolStore *SymbolStore) ([]ScoredResult, error) {
	return v.search(queryVector, topK, symbolStore, true)
}

// SearchByDistance performs similarity search using L2 distance
func (v *VectorStore) SearchByDistance(queryVector []float32, topK int, symbolStore *SymbolStore) ([]ScoredResult, error) {
	return v.search(queryVector, topK, symbolStore, false)
}

// search is the internal search implementation
func (v *VectorStore) search(queryVector []float32, topK int, symbolStore *SymbolStore, useSimilarity bool) ([]ScoredResult, error) {
	if len(queryVector) == 0 {
		return nil, fmt.Errorf("query vector is empty")
	}

	// Retrieve all vectors (for small datasets)
	// TODO: For larger datasets, implement approximate nearest neighbor (ANN) indexing
	query := "SELECT symbol_id, vector, dimension FROM embeddings"
	rows, err := v.db.sqlDB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query vectors: %w", err)
	}
	defer rows.Close()

	results := make([]ScoredResult, 0, topK)

	for rows.Next() {
		var symbolID string
		var blob []byte
		var dimension int

		if err := rows.Scan(&symbolID, &blob, &dimension); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		vector, err := blobToVector(blob)
		if err != nil {
			continue // Skip malformed vectors
		}

		// Skip dimension mismatch
		if len(vector) != len(queryVector) {
			continue
		}

		var score float32
		var distance float32

		if useSimilarity {
			score = embedding.Similarity(queryVector, vector)
			// Convert similarity to distance for ranking (higher similarity = lower distance)
			distance = 1 - score
		} else {
			distance = embedding.L2Distance(queryVector, vector)
			score = 1 / (1 + distance) // Convert distance to similarity-like score
		}

		results = append(results, ScoredResult{
			SymbolID: symbolID,
			Score:    score,
			Distance: distance,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	// Sort by score (descending)
	sortResults(results)

	// Keep top K
	if len(results) > topK {
		results = results[:topK]
	}

	// Optionally load full symbols
	if symbolStore != nil {
		symbolsMap := make(map[string]*Symbol)
		for i := range results {
			if sym, ok := symbolsMap[results[i].SymbolID]; ok {
				results[i].Symbol = sym
				continue
			}

			sym, err := symbolStore.GetByID(results[i].SymbolID)
			if err == nil {
				results[i].Symbol = sym
				symbolsMap[results[i].SymbolID] = sym
			}
		}
	}

	return results, nil
}

// SearchByFilters performs similarity search with additional filters
func (v *VectorStore) SearchByFilters(queryVector []float32, topK int, filters SearchFilters, symbolStore *SymbolStore) ([]ScoredResult, error) {
	// First get all results
	results, err := v.Search(queryVector, topK*2, symbolStore) // Get more to filter
	if err != nil {
		return nil, err
	}

	// Apply filters
	filtered := make([]ScoredResult, 0, topK)
	for _, result := range results {
		if result.Symbol == nil {
			continue
		}

		// Apply kind filter
		if len(filters.Kinds) > 0 {
			kindMatch := false
			for _, kind := range filters.Kinds {
				if result.Symbol.Kind == kind {
					kindMatch = true
					break
				}
			}
			if !kindMatch {
				continue
			}
		}

		// Apply exported filter
		if filters.ExportedyOnly && !result.Symbol.Exported {
			continue
		}

		// Apply package path filter
		if filters.PackagePath != "" && result.Symbol.PackagePath != filters.PackagePath {
			continue
		}

		filtered = append(filtered, result)

		if len(filtered) >= topK {
			break
		}
	}

	return filtered, nil
}

// Delete removes a vector
func (v *VectorStore) Delete(symbolID string) error {
	query := "DELETE FROM embeddings WHERE symbol_id = ?"
	_, err := v.db.sqlDB.Exec(query, symbolID)
	if err != nil {
		return fmt.Errorf("failed to delete vector: %w", err)
	}
	return nil
}

// DeleteByPrefix removes all vectors with symbol IDs matching a prefix
func (v *VectorStore) DeleteByPrefix(prefix string) error {
	query := "DELETE FROM embeddings WHERE symbol_id LIKE ?"
	_, err := v.db.sqlDB.Exec(query, prefix+"%")
	if err != nil {
		return fmt.Errorf("failed to delete vectors by prefix: %w", err)
	}
	return nil
}

// DeleteByRepo removes all vectors for symbols belonging to a repository.
func (v *VectorStore) DeleteByRepo(repoPath string) error {
	query := `
		DELETE FROM embeddings
		WHERE symbol_id IN (SELECT id FROM symbols WHERE repo_path = ?)
	`
	_, err := v.db.sqlDB.Exec(query, repoPath)
	if err != nil {
		return fmt.Errorf("failed to delete vectors by repo: %w", err)
	}
	return nil
}

// Count returns the number of vectors stored
func (v *VectorStore) Count() (int, error) {
	var count int
	err := v.db.sqlDB.QueryRow("SELECT COUNT(*) FROM embeddings").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count vectors: %w", err)
	}
	return count, nil
}

// HasVector checks if a symbol has a vector
func (v *VectorStore) HasVector(symbolID string) (bool, error) {
	var count int
	err := v.db.sqlDB.QueryRow("SELECT COUNT(*) FROM embeddings WHERE symbol_id = ?", symbolID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check vector: %w", err)
	}
	return count > 0, nil
}

// SearchFilters provides filtering options for vector search
type SearchFilters struct {
	Kinds         []string // Filter by symbol kinds
	ExportedyOnly bool     // Only exported symbols
	PackagePath   string   // Filter by package path
}

// Helper functions for vector serialization

// vectorToBlob converts a float32 slice to a binary blob
func vectorToBlob(vector []float32) ([]byte, error) {
	blob := make([]byte, len(vector)*4)
	for i, v := range vector {
		bits := math.Float32bits(v)
		binary.LittleEndian.PutUint32(blob[i*4:i*4+4], bits)
	}
	return blob, nil
}

// blobToVector converts a binary blob to a float32 slice
func blobToVector(blob []byte) ([]float32, error) {
	if len(blob)%4 != 0 {
		return nil, fmt.Errorf("blob size %d is not a multiple of 4", len(blob))
	}

	vector := make([]float32, len(blob)/4)
	for i := 0; i < len(vector); i++ {
		bits := binary.LittleEndian.Uint32(blob[i*4 : i*4+4])
		vector[i] = math.Float32frombits(bits)
	}

	return vector, nil
}

// sortResults sorts results by score (descending) using insertion sort
func sortResults(results []ScoredResult) {
	for i := 1; i < len(results); i++ {
		key := results[i]
		j := i - 1
		for j >= 0 && results[j].Score < key.Score {
			results[j+1] = results[j]
			j--
		}
		results[j+1] = key
	}
}
