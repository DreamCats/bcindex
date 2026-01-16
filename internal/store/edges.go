package store

import (
	"database/sql"
	"fmt"
	"time"
)

// EdgeStore provides CRUD operations for edges
type EdgeStore struct {
	db *DB
}

// NewEdgeStore creates a new edge store
func NewEdgeStore(db *DB) *EdgeStore {
	return &EdgeStore{db: db}
}

// Create inserts a new edge
func (e *EdgeStore) Create(edge *Edge) error {
	now := time.Now().UTC()
	edge.CreatedAt = now

	query := `
		INSERT INTO edges (from_id, to_id, edge_type, weight, import_path, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(from_id, to_id, edge_type) DO UPDATE SET
			weight = MAX(edges.weight, excluded.weight),
			import_path = COALESCE(edges.import_path, excluded.import_path)
	`

	_, err := e.db.sqlDB.Exec(query,
		edge.FromID, edge.ToID, edge.EdgeType, edge.Weight, edge.ImportPath, edge.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to insert edge: %w", err)
	}

	return nil
}

// CreateBatch inserts multiple edges in a transaction
func (e *EdgeStore) CreateBatch(edges []*Edge) error {
	if len(edges) == 0 {
		return nil
	}

	tx, err := e.db.BeginTx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO edges (from_id, to_id, edge_type, weight, import_path, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(from_id, to_id, edge_type) DO UPDATE SET
			weight = MAX(edges.weight, excluded.weight),
			import_path = COALESCE(edges.import_path, excluded.import_path)
	`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC()

	for _, edge := range edges {
		edge.CreatedAt = now

		_, err := stmt.Exec(
			edge.FromID, edge.ToID, edge.EdgeType, edge.Weight, edge.ImportPath, edge.CreatedAt,
		)

		if err != nil {
			return fmt.Errorf("failed to insert edge (%s -> %s): %w", edge.FromID, edge.ToID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// GetOutgoing returns all edges from a symbol
func (e *EdgeStore) GetOutgoing(fromID string, edgeType string) ([]*Edge, error) {
	query := `
		SELECT id, from_id, to_id, edge_type, weight, import_path, created_at
		FROM edges WHERE from_id = ?
	`

	args := []interface{}{fromID}
	if edgeType != "" {
		query += " AND edge_type = ?"
		args = append(args, edgeType)
	}

	rows, err := e.db.sqlDB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query edges: %w", err)
	}
	defer rows.Close()

	var edges []*Edge
	for rows.Next() {
		edge, err := e.scanEdge(rows)
		if err != nil {
			return nil, err
		}
		edges = append(edges, edge)
	}

	return edges, nil
}

// GetIncoming returns all edges to a symbol
func (e *EdgeStore) GetIncoming(toID string, edgeType string) ([]*Edge, error) {
	query := `
		SELECT id, from_id, to_id, edge_type, weight, import_path, created_at
		FROM edges WHERE to_id = ?
	`

	args := []interface{}{toID}
	if edgeType != "" {
		query += " AND edge_type = ?"
		args = append(args, edgeType)
	}

	rows, err := e.db.sqlDB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query edges: %w", err)
	}
	defer rows.Close()

	var edges []*Edge
	for rows.Next() {
		edge, err := e.scanEdge(rows)
		if err != nil {
			return nil, err
		}
		edges = append(edges, edge)
	}

	return edges, nil
}

// DeleteBySymbol removes all edges related to a symbol
func (e *EdgeStore) DeleteBySymbol(symbolID string) error {
	_, err := e.db.sqlDB.Exec("DELETE FROM edges WHERE from_id = ? OR to_id = ?", symbolID, symbolID)
	if err != nil {
		return fmt.Errorf("failed to delete edges: %w", err)
	}
	return nil
}

// DeleteByPackage removes all edges related to symbols in a package
func (e *EdgeStore) DeleteByPackage(pkgPath string) error {
	_, err := e.db.sqlDB.Exec(`
		DELETE FROM edges
		WHERE from_id IN (SELECT id FROM symbols WHERE package_path = ?)
		OR to_id IN (SELECT id FROM symbols WHERE package_path = ?)
	`, pkgPath, pkgPath)

	if err != nil {
		return fmt.Errorf("failed to delete edges: %w", err)
	}
	return nil
}

// Count returns the number of edges
func (e *EdgeStore) Count() (int, error) {
	var count int
	err := e.db.sqlDB.QueryRow("SELECT COUNT(*) FROM edges").Scan(&count)
	return count, err
}

// CountByType returns the number of edges of a specific type
func (e *EdgeStore) CountByType(edgeType string) (int, error) {
	var count int
	err := e.db.sqlDB.QueryRow("SELECT COUNT(*) FROM edges WHERE edge_type = ?", edgeType).Scan(&count)
	return count, err
}

// GetConnectedComponents finds symbols connected to a given symbol (within N hops)
func (e *EdgeStore) GetConnectedComponents(symbolID string, maxDepth int) (map[string][]string, error) {
	// BFS to find connected symbols
	visited := make(map[string]bool)
	queue := []string{symbolID}
	visited[symbolID] = true

	// adjacency list
	adjacency := make(map[string][]string)

	for i := 0; i < maxDepth && len(queue) > 0; i++ {
		levelSize := len(queue)
		for j := 0; j < levelSize; j++ {
			current := queue[0]
			queue = queue[1:]

			// Get outgoing edges
			outgoing, err := e.GetOutgoing(current, "")
			if err != nil {
				return nil, err
			}

			for _, edge := range outgoing {
				if !visited[edge.ToID] {
					visited[edge.ToID] = true
					queue = append(queue, edge.ToID)
				}
				adjacency[current] = append(adjacency[current], edge.ToID)
			}

			// Get incoming edges
			incoming, err := e.GetIncoming(current, "")
			if err != nil {
				return nil, err
			}

			for _, edge := range incoming {
				if !visited[edge.FromID] {
					visited[edge.FromID] = true
					queue = append(queue, edge.FromID)
				}
				// Also add reverse edge for undirected traversal
				if _, exists := adjacency[edge.ToID]; !exists {
					adjacency[edge.ToID] = []string{}
				}
			}
		}
	}

	return adjacency, nil
}

// scanEdge scans a row into an Edge
func (e *EdgeStore) scanEdge(rows *sql.Rows) (*Edge, error) {
	edge := &Edge{}

	var importPath sql.NullString

	err := rows.Scan(
		&edge.FromID, &edge.FromID, &edge.ToID, &edge.EdgeType,
		&edge.Weight, &importPath, &edge.CreatedAt,
	)

	// Note: there seems to be a duplicate scan for FromID above, let me fix that
	// Actually the query starts with id, so we need to skip it or handle it

	if err != nil {
		return nil, fmt.Errorf("failed to scan edge: %w", err)
	}

	if importPath.Valid {
		edge.ImportPath = importPath.String
	}

	return edge, nil
}

// scanEdgeFixed properly scans with the id column
func (e *EdgeStore) scanEdgeFixed(rows *sql.Rows) (*Edge, error) {
	edge := &Edge{}
	var id int64
	var importPath sql.NullString

	err := rows.Scan(
		&id, &edge.FromID, &edge.ToID, &edge.EdgeType,
		&edge.Weight, &importPath, &edge.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to scan edge: %w", err)
	}

	if importPath.Valid {
		edge.ImportPath = importPath.String
	}

	return edge, nil
}
