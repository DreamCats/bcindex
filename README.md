# BCIndex (MVP)

本项目提供本地 Go 仓库的索引与检索能力（符号 + 文本），以 CLI 方式使用。

## 功能范围（当前）
- Go 符号索引（函数、方法、结构体、接口、变量、常量）
- Markdown 文本分块索引
- 文本检索与符号检索（支持 mixed 简单融合）
- 本地用户目录持久化（`~/.bcindex/`）

不包含：
- 向量检索
- watch/增量索引
- MCP stdio API

## 快速开始

1) 初始化与全量索引
```bash
go run ./cmd/bcindex init --root .
go run ./cmd/bcindex index --root . --full --progress
```

1.1) 增量索引（基于 git diff）
```bash
go run ./cmd/bcindex index --root . --diff HEAD~1 --progress
```

1.2) 监听模式（轮询）
```bash
go run ./cmd/bcindex watch --root . --interval 3s --debounce 2s --progress
```

2) 查询示例
```bash
go run ./cmd/bcindex query --root . --q "IndexRepo" --type symbol
go run ./cmd/bcindex query --root . --q "BCIndex" --type text
go run ./cmd/bcindex query --root . --q "IndexRepo" --type mixed
go run ./cmd/bcindex query --root . --q "IndexRepo" --type mixed --json
go run ./cmd/bcindex query --root . --q "IndexRepo" --type mixed --progress
```

3) 查看状态
```bash
go run ./cmd/bcindex status --root .
```

4) 版本号
```bash
go run ./cmd/bcindex version
```

## 目录结构

索引数据默认存放于：
```
~/.bcindex/
  repos/<repo_id>/
    text/      # Bleve 文本索引
    symbol/    # SQLite symbols.db
    meta/      # repo.json
```

## 命令说明

```
bcindex init   --root <repo>
bcindex index  --root <repo> [--full|--diff <rev>] [--progress]
bcindex watch  --root <repo> [--interval 3s] [--debounce 2s] [--progress]
bcindex query  --root <repo> --q <text> --type <text|symbol|mixed> [--json] [--progress]
bcindex status --root <repo>
bcindex version [--root <repo>]
bcindex config init [--force]
```

## 常见问题

1) `--root` 未指定
- CLI 会从当前目录向上查找最近的 `.git` 作为仓库根目录。
- 若未找到，请显式传入 `--root`。

2) `go run ./bcindex ...` 报错
- 可执行入口在 `cmd/bcindex`，请使用：
  - `go run ./cmd/bcindex ...`

3) 向量化（Phase 1 配置文件）

配置文件路径：
```
~/.bcindex/config/bcindex.yaml
```

初始化配置：
```bash
go run ./cmd/bcindex config init
```

示例（最简，类似 docs-hub）：
```yaml
qdrant_path: "~/.bcindex/qdrant"
qdrant_collection: "bcindex_vectors"
qdrant_auto_start: true
volces_endpoint: "https://ark.cn-beijing.volces.com/api/v3/embeddings/multimodal"
volces_api_key: "your_api_key"
volces_model: "your_model_id"
volces_dimensions: 1024
volces_encoding: "float"
volces_timeout: "30s"
```

可选字段（需要时再填）：
```yaml
qdrant_url: "http://127.0.0.1:6333"
qdrant_api_key: ""
qdrant_bin: "qdrant"          # 不填则使用 PATH 中的 qdrant
qdrant_http_port: 6333
qdrant_grpc_port: 6334
volces_instructions: ""
```

## 文档参考
- `reference/BCINDEX_GO_TECH_SPEC.md`
- `reference/BCINDEX_MVP_TASKS.md`
- `reference/BCINDEX_DESIGN.md`
