package bcindex

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

type SymbolStore struct {
	db *sql.DB
}

func OpenSymbolStore(path string) (*SymbolStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	return &SymbolStore{db: db}, nil
}

func (s *SymbolStore) Close() error {
	return s.db.Close()
}

func (s *SymbolStore) InitSchema(reset bool) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS symbols (
			id INTEGER PRIMARY KEY,
			name TEXT,
			kind TEXT,
			file TEXT,
			line INTEGER,
			pkg TEXT,
			recv TEXT,
			doc TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS refs (
			symbol_id INTEGER,
			file TEXT,
			line INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS files (
			path TEXT PRIMARY KEY,
			hash TEXT,
			lang TEXT,
			size INTEGER,
			mtime INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS text_docs (
			file TEXT,
			doc_id TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS vector_docs (
			file TEXT,
			vector_id TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS relations (
			id INTEGER PRIMARY KEY,
			from_ref TEXT,
			to_ref TEXT,
			kind TEXT,
			file TEXT,
			line INTEGER,
			confidence REAL,
			source TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS relations_file_idx ON relations(file);`,
		`CREATE INDEX IF NOT EXISTS relations_kind_idx ON relations(kind);`,
		`CREATE TABLE IF NOT EXISTS doc_links (
			id INTEGER PRIMARY KEY,
			symbol TEXT,
			file TEXT,
			line INTEGER,
			confidence REAL,
			source TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS doc_links_file_idx ON doc_links(file);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("init schema: %w", err)
		}
	}
	if reset {
		resetStmts := []string{
			`DELETE FROM symbols;`,
			`DELETE FROM refs;`,
			`DELETE FROM files;`,
			`DELETE FROM text_docs;`,
			`DELETE FROM vector_docs;`,
			`DELETE FROM relations;`,
			`DELETE FROM doc_links;`,
		}
		for _, stmt := range resetStmts {
			if _, err := s.db.Exec(stmt); err != nil {
				return fmt.Errorf("reset schema: %w", err)
			}
		}
	}
	return nil
}

func (s *SymbolStore) InsertSymbol(sym Symbol) error {
	_, err := s.db.Exec(
		`INSERT INTO symbols (name, kind, file, line, pkg, recv, doc) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sym.Name, sym.Kind, sym.File, sym.Line, sym.Pkg, sym.Recv, sym.Doc,
	)
	if err != nil {
		return fmt.Errorf("insert symbol: %w", err)
	}
	return nil
}

func (s *SymbolStore) InsertRelation(rel Relation) error {
	_, err := s.db.Exec(
		`INSERT INTO relations (from_ref, to_ref, kind, file, line, confidence, source) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rel.FromRef, rel.ToRef, rel.Kind, rel.File, rel.Line, rel.Confidence, rel.Source,
	)
	if err != nil {
		return fmt.Errorf("insert relation: %w", err)
	}
	return nil
}

func (s *SymbolStore) InsertDocLink(link DocLink, file string) error {
	_, err := s.db.Exec(
		`INSERT INTO doc_links (symbol, file, line, confidence, source) VALUES (?, ?, ?, ?, ?)`,
		link.Symbol, file, link.Line, link.Confidence, link.Source,
	)
	if err != nil {
		return fmt.Errorf("insert doc link: %w", err)
	}
	return nil
}

func (s *SymbolStore) InsertFile(file FileEntry) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO files (path, hash, lang, size, mtime) VALUES (?, ?, ?, ?, ?)`,
		file.Path, file.Hash, file.Lang, file.Size, file.Mtime,
	)
	if err != nil {
		return fmt.Errorf("insert file: %w", err)
	}
	return nil
}

func (s *SymbolStore) DeleteFile(path string) error {
	_, err := s.db.Exec(`DELETE FROM files WHERE path = ?`, path)
	if err != nil {
		return fmt.Errorf("delete file: %w", err)
	}
	return nil
}

func (s *SymbolStore) DeleteSymbolsByFile(path string) error {
	_, err := s.db.Exec(`DELETE FROM symbols WHERE file = ?`, path)
	if err != nil {
		return fmt.Errorf("delete symbols: %w", err)
	}
	return nil
}

func (s *SymbolStore) DeleteRelationsByFile(path string) error {
	_, err := s.db.Exec(`DELETE FROM relations WHERE file = ?`, path)
	if err != nil {
		return fmt.Errorf("delete relations: %w", err)
	}
	return nil
}

func (s *SymbolStore) DeleteRelationsByKind(kind string) error {
	_, err := s.db.Exec(`DELETE FROM relations WHERE kind = ?`, kind)
	if err != nil {
		return fmt.Errorf("delete relations by kind: %w", err)
	}
	return nil
}

func (s *SymbolStore) DeleteDocLinksByFile(path string) error {
	_, err := s.db.Exec(`DELETE FROM doc_links WHERE file = ?`, path)
	if err != nil {
		return fmt.Errorf("delete doc links: %w", err)
	}
	return nil
}

func (s *SymbolStore) InsertTextDoc(file, docID string) error {
	_, err := s.db.Exec(`INSERT INTO text_docs (file, doc_id) VALUES (?, ?)`, file, docID)
	if err != nil {
		return fmt.Errorf("insert text doc: %w", err)
	}
	return nil
}

func (s *SymbolStore) DeleteTextDocs(file string) error {
	_, err := s.db.Exec(`DELETE FROM text_docs WHERE file = ?`, file)
	if err != nil {
		return fmt.Errorf("delete text docs: %w", err)
	}
	return nil
}

func (s *SymbolStore) ListTextDocIDs(file string) ([]string, error) {
	rows, err := s.db.Query(`SELECT doc_id FROM text_docs WHERE file = ?`, file)
	if err != nil {
		return nil, fmt.Errorf("list text docs: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan text doc: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *SymbolStore) SearchFilesByName(name string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 5
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}

	var rows *sql.Rows
	var err error
	if strings.Contains(name, "/") {
		pattern := "%" + name
		rows, err = s.db.Query(
			`SELECT path FROM files WHERE path = ? OR path LIKE ? ORDER BY path LIMIT ?`,
			name, pattern, limit,
		)
	} else {
		pattern := "%/" + name
		rows, err = s.db.Query(
			`SELECT path FROM files WHERE path = ? OR path LIKE ? ORDER BY path LIMIT ?`,
			name, pattern, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("search files: %w", err)
	}
	defer rows.Close()

	var files []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		files = append(files, path)
	}
	return files, nil
}

func (s *SymbolStore) InsertVectorDoc(file, vectorID string) error {
	_, err := s.db.Exec(`INSERT INTO vector_docs (file, vector_id) VALUES (?, ?)`, file, vectorID)
	if err != nil {
		return fmt.Errorf("insert vector doc: %w", err)
	}
	return nil
}

func (s *SymbolStore) ListVectorIDs(file string) ([]string, error) {
	rows, err := s.db.Query(`SELECT vector_id FROM vector_docs WHERE file = ?`, file)
	if err != nil {
		return nil, fmt.Errorf("list vector ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan vector id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *SymbolStore) DeleteVectorDocs(file string) error {
	_, err := s.db.Exec(`DELETE FROM vector_docs WHERE file = ?`, file)
	if err != nil {
		return fmt.Errorf("delete vector docs: %w", err)
	}
	return nil
}

func (s *SymbolStore) CountRelations() (int, error) {
	row := s.db.QueryRow(`SELECT COUNT(1) FROM relations`)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *SymbolStore) CountDocLinks() (int, error) {
	row := s.db.QueryRow(`SELECT COUNT(1) FROM doc_links`)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *SymbolStore) CountRelationsByKind(kind string) (int, error) {
	row := s.db.QueryRow(`SELECT COUNT(1) FROM relations WHERE kind = ?`, kind)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *SymbolStore) CountSymbols() (int, error) {
	row := s.db.QueryRow(`SELECT COUNT(1) FROM symbols`)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *SymbolStore) ListTopRelationPairs(kind string, limit int) ([]RelationPairStat, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(
		`SELECT from_ref, to_ref, COUNT(1) AS cnt
		 FROM relations
		 WHERE kind = ?
		 GROUP BY from_ref, to_ref
		 ORDER BY cnt DESC, from_ref, to_ref
		 LIMIT ?`,
		kind, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list relation pairs: %w", err)
	}
	defer rows.Close()

	var pairs []RelationPairStat
	for rows.Next() {
		var pair RelationPairStat
		if err := rows.Scan(&pair.FromRef, &pair.ToRef, &pair.Count); err != nil {
			return nil, fmt.Errorf("scan relation pair: %w", err)
		}
		pairs = append(pairs, pair)
	}
	return pairs, nil
}

func (s *SymbolStore) ListRelationsByFile(file string, limit int) ([]Relation, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT from_ref, to_ref, kind, file, line, confidence, source
		 FROM relations
		 WHERE file = ?
		 ORDER BY kind, line
		 LIMIT ?`,
		file, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list relations: %w", err)
	}
	defer rows.Close()

	var rels []Relation
	for rows.Next() {
		var rel Relation
		if err := rows.Scan(&rel.FromRef, &rel.ToRef, &rel.Kind, &rel.File, &rel.Line, &rel.Confidence, &rel.Source); err != nil {
			return nil, fmt.Errorf("scan relation: %w", err)
		}
		rels = append(rels, rel)
	}
	return rels, nil
}

func (s *SymbolStore) ListDocLinksByFileRange(file string, start, end, limit int) ([]DocLink, error) {
	if limit <= 0 {
		limit = 50
	}
	if start <= 0 {
		start = 1
	}
	if end <= 0 || end < start {
		end = start
	}
	rows, err := s.db.Query(
		`SELECT symbol, line, confidence, source
		 FROM doc_links
		 WHERE file = ? AND line BETWEEN ? AND ?
		 ORDER BY line
		 LIMIT ?`,
		file, start, end, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list doc links: %w", err)
	}
	defer rows.Close()

	var links []DocLink
	for rows.Next() {
		var link DocLink
		if err := rows.Scan(&link.Symbol, &link.Line, &link.Confidence, &link.Source); err != nil {
			return nil, fmt.Errorf("scan doc link: %w", err)
		}
		links = append(links, link)
	}
	return links, nil
}

func (s *SymbolStore) SearchSymbols(query string, limit int) ([]Symbol, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.Query(
		`SELECT name, kind, file, line, pkg, recv, doc
		 FROM symbols
		 WHERE name LIKE ?
		 ORDER BY CASE WHEN name = ? THEN 0 ELSE 1 END, length(name), name
		 LIMIT ?`,
		query+"%", query, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query symbols: %w", err)
	}
	defer rows.Close()

	var symbols []Symbol
	for rows.Next() {
		var sym Symbol
		if err := rows.Scan(&sym.Name, &sym.Kind, &sym.File, &sym.Line, &sym.Pkg, &sym.Recv, &sym.Doc); err != nil {
			return nil, fmt.Errorf("scan symbol: %w", err)
		}
		symbols = append(symbols, sym)
	}
	return symbols, nil
}
