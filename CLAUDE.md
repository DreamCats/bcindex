# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

BCIndex is a local Go repository indexing and retrieval system that combines multiple search strategies (keyword, symbol, and vector) to provide comprehensive code understanding capabilities. It's designed as a CLI-first tool for developers to quickly search and understand their codebases.

**Key capabilities:**
- Go symbol indexing (functions, methods, structs, interfaces, variables, constants)
- Function-level text chunking for Go code
- Markdown hierarchical chunking (title-based with overflow splitting)
- Relationship indexing (imports, package dependencies)
- Vector embeddings for semantic search (Qdrant + Volces)
- Tiered indexing (fast/balanced/full)
- Incremental indexing via git diff
- Watch mode for continuous updates

## Build and Run

```bash
# Install the CLI tool
go install ./cmd/bcindex

# Run directly
go run ./cmd/bcindex [command]
```

## Common Commands

```bash
# Initialize a repository (creates metadata and directory structure)
go run ./cmd/bcindex init --root /path/to/repo

# Full indexing (reset and rebuild all indexes)
go run ./cmd/bcindex index --root /path/to/repo --full --progress --tier fast

# Incremental indexing (based on git diff)
go run ./cmd/bcindex index --root /path/to/repo --diff HEAD~1 --progress

# Watch mode (poll with debounce)
go run ./cmd/bcindex watch --root /path/to/repo --interval 3s --debounce 2s --progress

# Query examples
go run ./cmd/bcindex query --root /path/to/repo --q "search term" --type text
go run ./cmd/bcindex query --root /path/to/repo --q "functionName" --type symbol
go run ./cmd/bcindex query --root /path/to/repo --q "question" --type mixed --mode context
go run ./cmd/bcindex query --root /path/to/repo --q "question" --type vector --mode architecture

# Check index status
go run ./cmd/bcindex status --root /path/to/repo

# Initialize/create config file
go run ./cmd/bcindex config init
```

**Note:** If `--root` is not specified, the CLI automatically finds the nearest `.git` directory starting from the current working directory.

## Architecture

### Core Components

**Entry Point:** `cmd/bcindex/main.go` → `internal/bcindex/cli.go`

**Indexing Pipeline** (`internal/bcindex/indexer.go`):
1. Parse Go files using `go/parser` and `go/ast`
2. Extract symbols and relationships (imports, dependencies)
3. Process Markdown files with hierarchical chunking
4. Build text index (Bleve) for keyword search
5. Build symbol index (SQLite) for precise symbol lookup
6. Generate embeddings for semantic search (if enabled)

**Query Engine** (`internal/bcindex/query.go`):
- Unified interface across all index types
- Supports: `text`, `symbol`, `vector`, `mixed` query types
- Output modes: `auto`, `search`, `context`, `impact`, `architecture`, `quality`
- Mixed queries combine results from all indexes with ranking

**Storage Layers:**
- **Text Index** (`text_index.go`): Bleve for full-text search with BM25/TF-IDF scoring
- **Symbol Index** (`go_symbols.go`, `symbol_store.go`): SQLite for Go symbols and references
- **Vector Index** (`vector_store.go`, `qdrant_client.go`): Qdrant for semantic embeddings
- **Metadata** (`repo.go`): JSON for repository state and timestamps

### File Organization

```
internal/bcindex/
├── cli.go                  # Command-line interface and command handlers
├── indexer.go              # Main indexing orchestration
├── query*.go               # Query engine and all query modes
├── go_symbols.go           # Go symbol extraction via AST
├── go_deps.go              # Package dependency resolution
├── markdown.go             # Markdown parsing and chunking
├── text_index.go           # Bleve text index operations
├── symbol_store.go         # SQLite symbol store operations
├── vector_store.go         # Vector store abstraction
├── vector_*.go             # Vector indexing and search
├── paths.go                # Path resolution utilities
├── repo.go                 # Repository metadata management
├── git.go                  # Git operations (diff, status)
├── types.go                # Core data structures
└── *_config.go             # Configuration loading and management
```

### Index Tiers

- **fast**: AST-only parsing (symbols + imports), fastest, default
- **balanced**: Adds package dependencies via `go list`
- **full**: Reserved for future enhancements (currently equivalent to balanced)

CLI `--tier` parameter takes precedence over config file.

### Query Types

- **text**: Keyword/path/regex search via Bleve text index
- **symbol**: Precise symbol lookup (functions, types, methods) via SQLite
- **vector**: Semantic search via embeddings
- **mixed**: Combines all three with intelligent ranking (default for most use cases)

### Query Modes

- **auto**: Automatically selects mode based on query intent (default)
- **search**: Direct search results with minimal formatting
- **context**: Combines relevant docs/code with relationship summaries, prioritizes docs for questions
- **impact**: Shows dependency/reference relationships for query results
- **architecture**: Repository relationship metrics and dependency graph
- **quality**: Index coverage statistics (symbols, relations, docs, text)

## Storage Layout

All index data is stored in the user home directory:

```
~/.bcindex/
├── config/
│   └── bcindex.yaml       # Main configuration file
└── repos/
    └── <repo_id>/         # Unique per repository (SHA-1 of root path)
        ├── text/          # Bleve text index
        ├── symbol/        # SQLite symbols.db
        ├── meta/          # repo.json metadata
        └── qdrant/        # Local Qdrant storage (if using local mode)
```

**Repository ID Generation**: `SHA1(root_path)` - ensures stable IDs across runs.

## Configuration

Config file location: `~/.bcindex/config/bcindex.yaml`

**Minimal configuration:**
```yaml
index:
  tier: "fast"
query:
  max_context_chars: 20000
qdrant_path: "~/.bcindex/qdrant"
qdrant_collection: "bcindex_vectors"
volces_endpoint: "https://ark.cn-beijing.volces.com/api/v3/embeddings/multimodal"
volces_api_key: "your_api_key"
volces_model: "your_model_id"
vector_enabled: true
```

**Vector mode selection:**
- If `qdrant_path` is set: Uses local embedded Qdrant (vectors.db in that directory)
- If `qdrant_url` is set: Connects to remote Qdrant service
- If neither is set: Vector indexing is disabled

**Optional configuration fields:**
```yaml
# Qdrant settings
qdrant_url: "http://127.0.0.1:6333"
qdrant_api_key: ""
qdrant_http_port: 6333
qdrant_grpc_port: 6334
qdrant_auto_start: true

# Volces embedding settings
volces_dimensions: 1024
volces_encoding: "float"
volces_timeout: "30s"
volces_instructions: ""

# Index tuning
vector_batch_size: 8
vector_max_chars: 1500
vector_workers: 4
vector_rerank_candidates: 300
vector_overlap_chars: 80
query_top_k: 10
```

## Key Design Patterns

1. **Repository-Centric**: Each repository has its own isolated index space. No cross-repo indexing or querying in the current implementation.

2. **Progressive Enhancement**: Start with fast indexing, incrementally add richer metadata and embeddings as needed.

3. **Index Coherency**: When indexing, existing indexes are cleared and rebuilt. For incremental updates, use `--diff` or watch mode.

4. **Error Handling**: Index operations use `IndexWarning` type - indexing continues even if individual files fail, with summary reported at completion.

5. **Query Variants**: Symbol search generates query variants (camelCase, snake_case, etc.) for better matching.

6. **Context Budgeting**: `context` mode respects `max_context_chars` limit to avoid overwhelming LLM context windows.

## Dependencies

- **Bleve v2.5.7**: Full-text search engine
- **SQLite (modernc.org/sqlite)**: Pure Go SQLite for symbol storage
- **Qdrant**: Vector database (local embedded or remote)
- **Volces Embeddings**: Vector embedding service
- **YAML (gopkg.in/yaml.v3)**: Configuration management
- **progressbar/v3**: CLI progress bars

## Working with the Codebase

### Adding a New Query Type

1. Add query type constant to `query.go`
2. Implement `query<NewType>()` function
3. Add handler in `QueryRepo()` switch statement
4. Update CLI help text in `cli.go`

### Modifying Indexing Behavior

1. Core indexing logic in `indexer.go:IndexRepoWithOptions()`
2. For Go-specific changes: `go_symbols.go` (extraction) or `go_deps.go` (dependencies)
3. For Markdown changes: `markdown.go`
4. Remember to update schema if storing new data in SQLite

### Testing Changes

No automated test suite currently exists. Manual testing workflow:

```bash
# Clean test: remove old index and rebuild
rm -rf ~/.bcindex/repos/<repo_id>
go run ./cmd/bcindex index --root . --full --progress

# Test query
go run ./cmd/bcindex query --root . --q "test query" --type mixed

# Test incremental
# Make code changes, then:
go run ./cmd/bcindex index --root . --diff HEAD~1 --progress
```

## Language and File Support

- **Go files** (`*.go`): Full AST parsing for symbols, function-level chunking for text
- **Markdown files** (`*.md`): Title-based hierarchical chunking
- **Other files**: Currently not indexed (future enhancement)

## Important Constraints

- Go 1.24.0+ required
- Git repository required (for `--root` auto-detection and incremental indexing)
- Vector features require external services (Volces) or local Qdrant
- Large repositories may take significant time for initial full index
