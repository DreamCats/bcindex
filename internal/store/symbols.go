package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// SymbolStore provides CRUD operations for symbols
type SymbolStore struct {
	db *DB
}

// NewSymbolStore creates a new symbol store
func NewSymbolStore(db *DB) *SymbolStore {
	return &SymbolStore{db: db}
}

// Create inserts a new symbol
func (s *SymbolStore) Create(sym *Symbol) error {
	now := time.Now().UTC()
	sym.CreatedAt = now
	sym.UpdatedAt = now

	// Serialize JSON fields
	tokensJSON, err := json.Marshal(sym.Tokens)
	if err != nil {
		return fmt.Errorf("failed to marshal tokens: %w", err)
	}

	typeDetailsJSON, err := json.Marshal(sym.TypeDetails)
	if err != nil {
		return fmt.Errorf("failed to marshal type_details: %w", err)
	}

	query := `
		INSERT INTO symbols (
			id, repo_path, kind, package_path, package_name, name, signature,
			file_path, line_start, line_end, doc_comment, exported, semantic_text,
			tokens, type_details, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.sqlDB.Exec(query,
		sym.ID, sym.RepoPath, sym.Kind, sym.PackagePath, sym.PackageName,
		sym.Name, sym.Signature, sym.FilePath, sym.LineStart, sym.LineEnd,
		sym.DocComment, boolToInt(sym.Exported), sym.SemanticText,
		string(tokensJSON), string(typeDetailsJSON), sym.CreatedAt, sym.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to insert symbol: %w", err)
	}

	return nil
}

// CreateBatch inserts multiple symbols in a transaction
func (s *SymbolStore) CreateBatch(syms []*Symbol) error {
	if len(syms) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO symbols (
			id, repo_path, kind, package_path, package_name, name, signature,
			file_path, line_start, line_end, doc_comment, exported, semantic_text,
			tokens, type_details, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now().UTC()

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, sym := range syms {
		sym.CreatedAt = now
		sym.UpdatedAt = now

		tokensJSON, err := json.Marshal(sym.Tokens)
		if err != nil {
			return fmt.Errorf("failed to marshal tokens: %w", err)
		}

		typeDetailsJSON, err := json.Marshal(sym.TypeDetails)
		if err != nil {
			return fmt.Errorf("failed to marshal type_details: %w", err)
		}

		_, err = stmt.Exec(
			sym.ID, sym.RepoPath, sym.Kind, sym.PackagePath, sym.PackageName,
			sym.Name, sym.Signature, sym.FilePath, sym.LineStart, sym.LineEnd,
			sym.DocComment, boolToInt(sym.Exported), sym.SemanticText,
			string(tokensJSON), string(typeDetailsJSON), sym.CreatedAt, sym.UpdatedAt,
		)

		if err != nil {
			return fmt.Errorf("failed to insert symbol %s: %w", sym.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// Get retrieves a symbol by ID
func (s *SymbolStore) Get(id string) (*Symbol, error) {
	query := `
		SELECT id, repo_path, kind, package_path, package_name, name, signature,
			file_path, line_start, line_end, doc_comment, exported, semantic_text,
			tokens, type_details, created_at, updated_at
		FROM symbols WHERE id = ?
	`

	row := s.db.sqlDB.QueryRow(query, id)
	sym, err := s.scanSymbolRow(row)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get symbol: %w", err)
	}

	return sym, nil
}

// GetByID is an alias for Get
func (s *SymbolStore) GetByID(id string) (*Symbol, error) {
	return s.Get(id)
}

// GetByPackage retrieves all symbols in a package
func (s *SymbolStore) GetByPackage(pkgPath string) ([]*Symbol, error) {
	query := `
		SELECT id, repo_path, kind, package_path, package_name, name, signature,
			file_path, line_start, line_end, doc_comment, exported, semantic_text,
			tokens, type_details, created_at, updated_at
		FROM symbols WHERE package_path = ?
		ORDER BY kind, name
	`

	rows, err := s.db.sqlDB.Query(query, pkgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to query symbols: %w", err)
	}
	defer rows.Close()

	var symbols []*Symbol
	for rows.Next() {
		sym, err := s.scanSymbolRow(rows)
		if err != nil {
			return nil, err
		}
		symbols = append(symbols, sym)
	}

	return symbols, nil
}

// GetByRepo retrieves all symbols in a repository
func (s *SymbolStore) GetByRepo(repoPath string) ([]*Symbol, error) {
	query := `
		SELECT id, repo_path, kind, package_path, package_name, name, signature,
			file_path, line_start, line_end, doc_comment, exported, semantic_text,
			tokens, type_details, created_at, updated_at
		FROM symbols WHERE repo_path = ?
		ORDER BY package_path, kind, name
	`

	rows, err := s.db.sqlDB.Query(query, repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to query symbols: %w", err)
	}
	defer rows.Close()

	var symbols []*Symbol
	for rows.Next() {
		sym, err := s.scanSymbolRow(rows)
		if err != nil {
			return nil, err
		}
		symbols = append(symbols, sym)
	}

	return symbols, nil
}

// FindByName retrieves symbols by exact name with optional repo/package filters.
func (s *SymbolStore) FindByName(name string, repoPath string, packagePath string, limit int) ([]*Symbol, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if limit <= 0 {
		limit = 20
	}

	query := `
		SELECT id, repo_path, kind, package_path, package_name, name, signature,
			file_path, line_start, line_end, doc_comment, exported, semantic_text,
			tokens, type_details, created_at, updated_at
		FROM symbols WHERE name = ?
	`
	args := []interface{}{name}

	if repoPath != "" {
		query += " AND repo_path = ?"
		args = append(args, repoPath)
	}
	if packagePath != "" {
		query += " AND package_path = ?"
		args = append(args, packagePath)
	}

	query += " ORDER BY package_path, kind, name LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.sqlDB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query symbols: %w", err)
	}
	defer rows.Close()

	var symbols []*Symbol
	for rows.Next() {
		sym, err := s.scanSymbolRow(rows)
		if err != nil {
			return nil, err
		}
		symbols = append(symbols, sym)
	}

	return symbols, nil
}

// FileSymbol represents a file-level symbol record.
type FileSymbol struct {
	PackagePath string
	FilePath    string
}

// ListFilesByRepo returns file symbols for a repository.
func (s *SymbolStore) ListFilesByRepo(repoPath string) ([]FileSymbol, error) {
	query := `
		SELECT package_path, file_path
		FROM symbols
		WHERE repo_path = ? AND kind = ?
	`

	rows, err := s.db.sqlDB.Query(query, repoPath, KindFile)
	if err != nil {
		return nil, fmt.Errorf("failed to query file symbols: %w", err)
	}
	defer rows.Close()

	var files []FileSymbol
	for rows.Next() {
		var fs FileSymbol
		if err := rows.Scan(&fs.PackagePath, &fs.FilePath); err != nil {
			return nil, fmt.Errorf("failed to scan file symbol: %w", err)
		}
		files = append(files, fs)
	}

	return files, nil
}

// Update updates a symbol's semantic text and tokens
func (s *SymbolStore) Update(id string, semanticText string, tokens []string) error {
	now := time.Now().UTC()

	tokensJSON, err := json.Marshal(tokens)
	if err != nil {
		return fmt.Errorf("failed to marshal tokens: %w", err)
	}

	query := `
		UPDATE symbols
		SET semantic_text = ?, tokens = ?, updated_at = ?
		WHERE id = ?
	`

	_, err = s.db.sqlDB.Exec(query, semanticText, string(tokensJSON), now, id)
	if err != nil {
		return fmt.Errorf("failed to update symbol: %w", err)
	}

	return nil
}

// Delete removes a symbol
func (s *SymbolStore) Delete(id string) error {
	_, err := s.db.sqlDB.Exec("DELETE FROM symbols WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete symbol: %w", err)
	}
	return nil
}

// DeleteByPackage removes all symbols in a package
func (s *SymbolStore) DeleteByPackage(pkgPath string) error {
	_, err := s.db.sqlDB.Exec("DELETE FROM symbols WHERE package_path = ?", pkgPath)
	if err != nil {
		return fmt.Errorf("failed to delete symbols: %w", err)
	}
	return nil
}

// DeleteByRepo removes all symbols in a repository
func (s *SymbolStore) DeleteByRepo(repoPath string) error {
	_, err := s.db.sqlDB.Exec("DELETE FROM symbols WHERE repo_path = ?", repoPath)
	if err != nil {
		return fmt.Errorf("failed to delete symbols: %w", err)
	}
	return nil
}

// Count returns the number of symbols
func (s *SymbolStore) Count() (int, error) {
	var count int
	err := s.db.sqlDB.QueryRow("SELECT COUNT(*) FROM symbols").Scan(&count)
	return count, err
}

// CountByRepo returns the number of symbols in a repository
func (s *SymbolStore) CountByRepo(repoPath string) (int, error) {
	var count int
	err := s.db.sqlDB.QueryRow("SELECT COUNT(*) FROM symbols WHERE repo_path = ?", repoPath).Scan(&count)
	return count, err
}

// SearchFTS performs full-text search on symbols
func (s *SymbolStore) SearchFTS(query string, limit int) ([]*Symbol, error) {
	if limit <= 0 {
		limit = 10
	}

	symbols, err := s.searchFTS(query, limit)
	if err == nil {
		return symbols, nil
	}
	if !isFTSSyntaxError(err) {
		return nil, err
	}

	cleaned := sanitizeFTSQuery(query)
	if cleaned == "" {
		return []*Symbol{}, nil
	}
	if cleaned == query {
		return nil, err
	}

	symbols, retryErr := s.searchFTS(cleaned, limit)
	if retryErr != nil {
		return nil, err
	}
	return symbols, nil
}

func (s *SymbolStore) searchFTS(query string, limit int) ([]*Symbol, error) {
	// Use FTS5 to search
	sqlQuery := `
		SELECT s.id, s.repo_path, s.kind, s.package_path, s.package_name,
		       s.name, s.signature, s.file_path, s.line_start, s.line_end,
		       s.doc_comment, s.exported, s.semantic_text, s.tokens, s.type_details,
		       s.created_at, s.updated_at
		FROM symbols_fts fts
		JOIN symbols s ON s.id = fts.id
		WHERE symbols_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`

	rows, err := s.db.sqlDB.Query(sqlQuery, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query symbols_fts: %w", err)
	}
	defer rows.Close()

	var symbols []*Symbol
	for rows.Next() {
		sym, err := s.scanSymbolRow(rows)
		if err != nil {
			return nil, err
		}
		symbols = append(symbols, sym)
	}

	return symbols, nil
}

func isFTSSyntaxError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "fts5:syntax error")
}

func sanitizeFTSQuery(query string) string {
	if strings.TrimSpace(query) == "" {
		return ""
	}

	var b strings.Builder
	lastSpace := true
	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}

	return strings.TrimSpace(b.String())
}

// scanSymbolRow scans a row into a Symbol
func (s *SymbolStore) scanSymbolRow(scanner rowScanner) (*Symbol, error) {
	sym := &Symbol{}
	var tokensJSON, typeDetailsJSON string
	var exported int
	var createdAtValue any
	var updatedAtValue any

	err := scanner.Scan(
		&sym.ID, &sym.RepoPath, &sym.Kind, &sym.PackagePath, &sym.PackageName,
		&sym.Name, &sym.Signature, &sym.FilePath, &sym.LineStart, &sym.LineEnd,
		&sym.DocComment, &exported, &sym.SemanticText,
		&tokensJSON, &typeDetailsJSON, &createdAtValue, &updatedAtValue,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to scan symbol: %w", err)
	}

	sym.Exported = intToBool(exported)
	createdAt, err := parseTimeValue(createdAtValue)
	if err != nil {
		return nil, fmt.Errorf("failed to parse created_at: %w", err)
	}
	updatedAt, err := parseTimeValue(updatedAtValue)
	if err != nil {
		return nil, fmt.Errorf("failed to parse updated_at: %w", err)
	}
	sym.CreatedAt = createdAt
	sym.UpdatedAt = updatedAt

	if tokensJSON != "" {
		if err := json.Unmarshal([]byte(tokensJSON), &sym.Tokens); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tokens: %w", err)
		}
	}

	if typeDetailsJSON != "" {
		if err := json.Unmarshal([]byte(typeDetailsJSON), &sym.TypeDetails); err != nil {
			return nil, fmt.Errorf("failed to unmarshal type_details: %w", err)
		}
	}

	return sym, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intToBool(i int) bool {
	return i != 0
}
