# BCIndex Go 技术文档

> 范围：基于 Go 的本地仓库索引与搜索能力（MVP 仅 CLI，MCP stdio 为后续阶段）。

## 1. 目标与非目标

目标：
- 为 Go 代码与 Markdown 建立高质量索引（符号 + 文本）。
- 提供本地 CLI；MCP stdio API 作为后续演进能力。
- 所有数据落地到用户级目录（如 `~/.bcindex/`）。
- MVP 仅支持全量索引，增量与监听作为后续阶段。

非目标（MVP）：
- MCP stdio API。
- 分布式索引。
- 极低延迟检索优化。
- 超出本地用户权限的复杂权限系统。

## 2. 架构总览

```
CLI / MCP (stdio)
        |
        v
Query Engine
  |   |   |
Text Symbol Vector(optional)
  |   |   |
 Index Stores (per repo)
        ^
        |
   Indexer (Go parser + Markdown)
        ^
        |
   Repo Watcher / CLI
```

## 3. 语言与运行时

- Go 1.21+（推荐 1.22）
- 单一静态二进制，便于安装与升级
- 目标系统：macOS/Linux（Windows 可后续支持）

## 4. 组件与职责

### 4.1 CLI

命令：
- `bcindex init --root <repo>`
- `bcindex index --root <repo> --full`
- `bcindex query --repo <name> --q "<query>" --type <text|symbol|mixed>`
- `bcindex status --repo <name>`

行为说明：
- `--root` 为空时，从当前目录向上查找最近的 `.git` 根目录。
- 首次 `index --full` 自动完成 `init`（若未初始化）。

### 4.2 仓库监听（后续阶段）

- MVP 不实现监听与增量索引。
- 后续可使用 `git status --porcelain` 或 `git diff --name-only` 获取变更文件。

### 4.3 索引器（Indexer）

#### Go 代码索引
- 使用 `go/parser`、`go/ast`、`go/token`。
- 抽取：包名、符号名、符号类型、接收者、注释、文件路径、行号。
- 可选使用 `go list` 识别模块边界与包关系。

#### Markdown 索引
- 按 `#`..`####` 标题分块。
- 存储：块文本、标题路径、文件路径、行号范围。

### 4.4 查询引擎

优先级顺序：
1) 符号精确/前缀匹配  
2) 文本检索（关键词/正则/路径）  
3) 向量检索（可选）  
4) 混合排序（BM25 + 符号权重 + 最近更新权重）

### 4.5 索引存储

每个仓库独立存储：
```
~/.bcindex/
  repos/<repo_id>/
    text/          # 文本索引
    symbol/        # SQLite symbols.db
    vector/        # 向量索引（可选）
    meta/          # repo.json, hashes
```

## 5. 数据模型

SQLite（symbols.db）：
```
symbols(
  id INTEGER PRIMARY KEY,
  name TEXT,
  kind TEXT,
  file TEXT,
  line INTEGER,
  pkg TEXT,
  recv TEXT,
  doc TEXT
)

refs(
  symbol_id INTEGER,
  file TEXT,
  line INTEGER
)

files(
  path TEXT PRIMARY KEY,
  hash TEXT,
  lang TEXT,
  size INTEGER,
  mtime INTEGER
)
```

元数据：
```
repo.json:
  repo_id, root, created_at, updated_at, last_commit
```

## 6. 文本索引方案

选项：
- Zoekt：高性能、代码搜索成熟方案。
- Bleve / SQLite FTS5：实现简单，适合 MVP。

MVP 建议：
- 先用 Bleve 或 SQLite FTS5，预留接口以便替换 Zoekt。

## 7. 向量索引（后续阶段）

向量来源（后续）：
- OpenAI Embedding（配置驱动）。
- 本地模型（后续可选）。

分块策略：
- Go：函数级块（签名 + 注释 + 片段）。
- Markdown：标题块。

存储方案（后续）：
- Qdrant 或 pgvector（本地）。

## 8. MCP stdio API（后续阶段）

MVP 不实现 MCP，保留扩展设计。后续可提供最小集合：
- `repo.list`
- `search`

`search` 请求字段：
```
query, repo, type, top_k
type = text | symbol | mixed
```

## 9. 配置

`~/.bcindex/config/bcindex.yaml`：
```
root_cache_dir: ~/.bcindex
default_top_k: 10
index:
  text: bleve
  vector: openai
openai:
  api_key_env: OPENAI_API_KEY
```

## 10. 错误处理

- 找不到 repo root：给出可操作提示（显式指定 `--root`）。
- 索引锁：同一仓库仅允许一个索引任务。
- 索引损坏：回退为全量重建。

## 11. 安全与合规

- 核心索引无网络访问需求。
- 向量化可按仓库开关，避免敏感代码外发。
- 所有数据仅存本地用户目录。
