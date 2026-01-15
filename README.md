# BCIndex (MVP)

本项目提供本地 Go 仓库的索引与检索能力（符号 + 文本），以 CLI 方式使用。

## 功能范围（当前）
- Go 符号索引（函数、方法、结构体、接口、变量、常量）
- Go 文本索引（函数/方法级分块）
- Markdown 文本分块索引（标题分块 + 超长段落自动拆分）
- 文本检索与符号检索（支持 mixed 简单融合）
- 向量索引写入（Qdrant + Volces embedding）
- 本地用户目录持久化（`~/.bcindex/`）
- 增量索引（基于 git diff）
- watch 监听模式（轮询 + 去抖/批处理）

不包含：
- 向量检索/混合检索（Phase 3）
- MCP stdio API

## 安装

在仓库根目录执行：
```bash
go install ./cmd/bcindex
```

## 快速开始

1) 初始化与全量索引
```bash
go run ./cmd/bcindex init --root .
go run ./cmd/bcindex index --root . --full --progress
```

说明：`index --full` 会自动初始化仓库元信息与目录，`init` 可选。

首次使用向量化时，若未创建配置文件，`index` 会自动生成默认配置并提示你补全 `volces_api_key` 与 `volces_model`，否则将降级为仅文本/符号索引。

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
go run ./cmd/bcindex query --root . --q "索引进度条如何实现" --type vector
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
bcindex query  --root <repo> --q <text> --type <text|symbol|mixed|vector> [--json] [--progress]
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

3) 向量化配置文件

配置文件路径：
```
~/.bcindex/config/bcindex.yaml
```

初始化配置：
```bash
go run ./cmd/bcindex config init
```

说明：
- `qdrant_path` 指定本地存储目录（本地模式，不依赖 Qdrant 进程）。
- 若 `qdrant_path` 为空，则使用 `qdrant_url` 连接远程 Qdrant 服务。
- 本地模式会将向量写入 `qdrant_path/vectors.db`。

示例（最简，类似 docs-hub）：
```yaml
qdrant_path: "~/.bcindex/qdrant"
qdrant_collection: "bcindex_vectors"
volces_endpoint: "https://ark.cn-beijing.volces.com/api/v3/embeddings/multimodal"
volces_api_key: "your_api_key"
volces_model: "your_model_id"
vector_enabled: true
```

可选字段（需要时再填）：
```yaml
qdrant_url: "http://127.0.0.1:6333"
qdrant_api_key: ""
qdrant_http_port: 6333
qdrant_grpc_port: 6334
volces_dimensions: 1024
volces_encoding: "float"
volces_timeout: "30s"
volces_instructions: ""
vector_batch_size: 8
vector_max_chars: 1500
vector_workers: 4
vector_rerank_candidates: 300
vector_overlap_chars: 80
query_top_k: 10
```

## 文档参考
- `reference/BCINDEX_GO_TECH_SPEC.md`
- `reference/BCINDEX_MVP_TASKS.md`
- `reference/BCINDEX_DESIGN.md`
