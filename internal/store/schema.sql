CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

-- Symbols table: stores all semantic units
CREATE TABLE IF NOT EXISTS symbols (
    id TEXT PRIMARY KEY,
    repo_path TEXT NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('package', 'file', 'interface', 'struct', 'func', 'method', 'const', 'var', 'field')),
    package_path TEXT NOT NULL,
    package_name TEXT NOT NULL,
    name TEXT NOT NULL,
    signature TEXT,
    file_path TEXT,
    line_start INTEGER,
    line_end INTEGER,
    doc_comment TEXT,
    exported INTEGER NOT NULL DEFAULT 0,
    semantic_text TEXT,
    tokens TEXT, -- JSON array of keywords
    type_details TEXT, -- JSON blob for TypeDetails
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Indexes for symbols
CREATE INDEX IF NOT EXISTS idx_symbols_repo ON symbols(repo_path);
CREATE INDEX IF NOT EXISTS idx_symbols_kind ON symbols(kind);
CREATE INDEX IF NOT EXISTS idx_symbols_package ON symbols(package_path);
CREATE INDEX IF NOT EXISTS idx_symbols_name ON symbols(name);
CREATE INDEX IF NOT EXISTS idx_symbols_exported ON symbols(exported);

-- Full-text search on symbol names and semantic_text
CREATE VIRTUAL TABLE IF NOT EXISTS symbols_fts USING fts5(
    id,
    name,
    semantic_text,
    content=symbols,
    content_rowid=rowid
);

-- Triggers to keep FTS in sync
CREATE TRIGGER IF NOT EXISTS symbols_fts_insert AFTER INSERT ON symbols BEGIN
    INSERT INTO symbols_fts(rowid, id, name, semantic_text)
    VALUES (new.rowid, new.id, new.name, new.semantic_text);
END;

CREATE TRIGGER IF NOT EXISTS symbols_fts_delete AFTER DELETE ON symbols BEGIN
    DELETE FROM symbols_fts WHERE rowid = old.rowid;
END;

CREATE TRIGGER IF NOT EXISTS symbols_fts_update AFTER UPDATE ON symbols BEGIN
    DELETE FROM symbols_fts WHERE rowid = old.rowid;
    INSERT INTO symbols_fts(rowid, id, name, semantic_text)
    VALUES (new.rowid, new.id, new.name, new.semantic_text);
END;

-- Edges table: stores relationships between symbols
CREATE TABLE IF NOT EXISTS edges (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_id TEXT NOT NULL,
    to_id TEXT NOT NULL,
    edge_type TEXT NOT NULL CHECK(edge_type IN ('calls', 'implements', 'imports', 'references', 'embeds')),
    weight INTEGER NOT NULL DEFAULT 1,
    import_path TEXT, -- For import edges
    created_at TEXT NOT NULL,
    FOREIGN KEY (from_id) REFERENCES symbols(id) ON DELETE CASCADE,
    FOREIGN KEY (to_id) REFERENCES symbols(id) ON DELETE CASCADE,
    UNIQUE(from_id, to_id, edge_type)
);

-- Indexes for edges
CREATE INDEX IF NOT EXISTS idx_edges_from ON edges(from_id);
CREATE INDEX IF NOT EXISTS idx_edges_to ON edges(to_id);
CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(edge_type);

-- Packages table: aggregated package information
CREATE TABLE IF NOT EXISTS packages (
    path TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    role TEXT,
    summary TEXT,
    key_types TEXT, -- JSON array
    key_funcs TEXT, -- JSON array
    interfaces TEXT, -- JSON array
    imports TEXT, -- JSON array
    imported_by TEXT, -- JSON array
    file_count INTEGER DEFAULT 0,
    symbol_count INTEGER DEFAULT 0,
    line_count INTEGER DEFAULT 0,
    repo_path TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Indexes for packages
CREATE INDEX IF NOT EXISTS idx_packages_repo ON packages(repo_path);
CREATE INDEX IF NOT EXISTS idx_packages_role ON packages(role);

-- Full-text search on packages
CREATE VIRTUAL TABLE IF NOT EXISTS packages_fts USING fts5(
    path,
    name,
    summary,
    content=packages,
    content_rowid=rowid
);

-- Triggers for packages FTS
CREATE TRIGGER IF NOT EXISTS packages_fts_insert AFTER INSERT ON packages BEGIN
    INSERT INTO packages_fts(rowid, path, name, summary)
    VALUES (new.rowid, new.path, new.name, new.summary);
END;

CREATE TRIGGER IF NOT EXISTS packages_fts_delete AFTER DELETE ON packages BEGIN
    DELETE FROM packages_fts WHERE rowid = old.rowid;
END;

CREATE TRIGGER IF NOT EXISTS packages_fts_update AFTER UPDATE ON packages BEGIN
    DELETE FROM packages_fts WHERE rowid = old.rowid;
    INSERT INTO packages_fts(rowid, path, name, summary)
    VALUES (new.rowid, new.path, new.name, new.summary);
END;

-- Vector embeddings table (optional, separate from main schema)
CREATE TABLE IF NOT EXISTS embeddings (
    symbol_id TEXT PRIMARY KEY,
    vector BLOB NOT NULL, -- Stored as binary (float32 array)
    dimension INTEGER NOT NULL,
    model TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (symbol_id) REFERENCES symbols(id) ON DELETE CASCADE
);

-- Repositories table: track indexed repositories
CREATE TABLE IF NOT EXISTS repositories (
    id TEXT PRIMARY KEY, -- SHA-1 of root path
    root_path TEXT UNIQUE NOT NULL,
    last_indexed_at TEXT,
    symbol_count INTEGER DEFAULT 0,
    package_count INTEGER DEFAULT 0,
    edge_count INTEGER DEFAULT 0,
    has_embeddings INTEGER DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Indexing jobs table: track indexing operations
CREATE TABLE IF NOT EXISTS indexing_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('pending', 'running', 'completed', 'failed')),
    started_at TEXT,
    completed_at TEXT,
    error TEXT,
    symbols_processed INTEGER DEFAULT 0,
    packages_processed INTEGER DEFAULT 0,
    files_processed INTEGER DEFAULT 0,
    FOREIGN KEY (repo_id) REFERENCES repositories(id) ON DELETE CASCADE
);
