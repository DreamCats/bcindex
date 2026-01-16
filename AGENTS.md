# Repository Guidelines

## Project Structure & Module Organization
- `cmd/`: CLI entrypoints (`bcindex`, `extract`, `relations`, `embed`).
- `internal/`: core packages (AST extraction, indexing, retrieval, embeddings, storage, config).
- `reference/`: architecture and design notes.
- `scripts/`: helper scripts (for example `scripts/install.sh`).
- `testdata/`: fixtures used by tests.
- `config.example.yaml`: sample configuration for embedding providers and storage.

## Build, Test, and Development Commands
- `make build`: builds `./bcindex` from `./cmd/bcindex`.
- `make test`: runs `go test -v ./...`.
- `make fmt`: formats with `go fmt ./...`.
- `make vet`: runs `go vet ./...`.
- `make lint`: runs `golangci-lint run` (install separately).
- `make clean`: removes `./bcindex` and clears `~/.bcindex/data/*`.
- Direct build: `go build -o bcindex ./cmd/bcindex`.
- Toolchain: Go 1.24 (see `go.mod`).

## Coding Style & Naming Conventions
- Use `gofmt` for indentation and formatting (tabs per Go conventions).
- Exported identifiers use `CamelCase`; unexported use `lowerCamel`.
- Package names are short, lowercase, and descriptive.
- Test files use `*_test.go`; fixtures live under `testdata/`.

## Testing Guidelines
- Standard runner is `go test ./...` or `make test`.
- Add unit tests for new indexing, retrieval, and config behavior.
- Use `testdata/` for stable fixtures instead of inline blobs.
- No explicit coverage threshold is defined; keep changes well-covered.

## Commit & Pull Request Guidelines
- Commit subjects typically follow conventional prefixes (e.g., `feat:`, `docs:`); keep them imperative and concise.
- Include a short “what/why” and a “how to test” section in PRs (for example, `make test`).
- Link related issues and update docs (`README.md`, `QUICKSTART.md`, `config.example.yaml`) when flags or config change.

## Configuration & Secrets
- Local config lives at `~/.bcindex/config/bcindex.yaml` (see `config.example.yaml`).
- Never commit API keys or credentials; keep them in local config or environment variables.
