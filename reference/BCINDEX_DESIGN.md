# 本地仓库索引与对外搜索能力设计（MVP + 可演进版）

> 目标：为本地 Go 仓库建立高质量上下文索引，并通过 CLI/Skill/MCP 对外提供检索能力。

## 1. 背景与目标

### 背景
- LLM 在缺少上下文时，需要大量 grep/正则搜索，效率低、token 成本高。
- 需要一个可本地落地、可扩展的索引机制，服务于编程协作与外部工具（Claude Code/Codex CLI 等）。

### 目标
- 支持 Go 代码与 Markdown 的检索与上下文组织。
- 索引与配置落地到用户级目录（示例：`~/.bcindex/`）。
- 提供 CLI + MCP/Skill 接口，打通 LLM 调用链路。
- MVP 允许较高延迟，后续支持增量/实时优化。

## 2. 总体架构

```
                 ┌───────────────────────────────┐
                 │        CLI / MCP / Skill      │
                 └───────────────────────────────┘
                              │
                              ▼
                 ┌───────────────────────────────┐
                 │        Query Engine           │
                 │  Text + Symbol + Vector       │
                 └───────────────────────────────┘
                              │
                              ▼
                 ┌───────────────────────────────┐
                 │          Index Store          │
                 │ Text / Symbol / Vector / Meta │
                 └───────────────────────────────┘
                              ▲
                              │
                 ┌───────────────────────────────┐
                 │           Indexer             │
                 │ Go Parser / Markdown Parser   │
                 └───────────────────────────────┘
                              ▲
                              │
                 ┌───────────────────────────────┐
                 │       Repo Watcher / CLI      │
                 └───────────────────────────────┘
```

## 3. 数据流与索引流程

1) 初始化仓库：`bcindex init --root /path/repo`  
2) 全量索引：`bcindex index --root /path/repo --full`  
3) 增量索引：`bcindex index --root /path/repo --diff HEAD~1`  
4) 后台监听：`bcindex watch --root /path/repo`  
5) 对外查询：`bcindex query --repo name --q "GetLiveProducts"`

索引流程（MVP）：
- Go：`go/parser` + `go/ast` 提取符号、文件、包信息。
- Markdown：按标题层级切分 chunk，记录标题链与链接。
- 文本索引：全文 + 关键词（BM25/TF-IDF）。
- 符号索引：函数/类型/方法/变量/常量。

## 4. 索引类型与检索策略

### 4.1 文本索引（Text Index）
用途：正则/关键词/路径搜索  
建议：Zoekt 或 Bleve/SQLite FTS5  

### 4.2 符号索引（Symbol Index）
用途：符号名、定义、引用  
建议：SQLite 表结构  

示例表结构（MVP）：
```
symbols(id, name, kind, file, line, pkg, recv, doc)
refs(symbol_id, file, line)
files(path, hash, lang, size, mtime)
```

### 4.3 向量索引（Vector Index）
用途：语义检索，提升 LLM 召回质量  
建议：Qdrant / Milvus / pgvector（单机优先 pgvector）  

向量化来源：
- Go 代码：函数级 chunk（包含签名 + 注释 + 局部上下文）
- Markdown：标题级 chunk

向量服务选择：
- OpenAI embedding（如 `text-embedding-3-large`）
- 本地模型（如 BGE-code、Jina embeddings）

向量字段：
```
vector(id, repo, path, kind, chunk_text, embedding, start_line, end_line, hash)
```

### 4.4 检索融合策略
优先级：  
1) 符号精确匹配  
2) 文本检索（路径/关键词）  
3) 向量检索  
4) 混合排序（BM25 + Vector + 结构权重）

## 5. 目录与存储布局

用户级目录建议：
```
~/.bcindex/
├── repos/
│   ├── <repo_id>/
│   │   ├── text/          # 文本索引
│   │   ├── symbol/        # SQLite (symbols.db)
│   │   ├── vector/        # 向量索引
│   │   └── meta/          # repo.json, hashes
├── cache/
│   ├── ast/               # 可选 AST/符号缓存
│   └── embeddings/        # 可选 embedding 缓存
└── config/
    └── bcindex.yaml
```

## 6. 每个仓库一套索引 vs 多仓混合索引

### 方案 A：每仓库独立索引（推荐）
优点：
- 逻辑清晰，数据隔离，易于维护与增量更新。
- 查询精度高，避免跨仓污染。
- 更适合本地个人多仓使用场景。

缺点：
- 多仓查询需要聚合逻辑。
- 同一符号在多个仓库重复存储。

### 方案 B：多仓混合索引
优点：
- 支持跨仓检索，统一入口。
- 统一向量检索可提升泛化 recall。

缺点：
- 索引冲突风险高（重名符号、同名文件）。
- 增量更新复杂，需维护 repo_id 过滤。
- 权限与隔离需要额外处理。

### 结论建议
- **MVP 使用“每仓独立索引”**，确保可维护性与准确度。  
- 后续可在 Query 层增加“多仓聚合检索”能力，不强制合并索引存储。

## 7. CLI / MCP / Skill 设计

### 7.1 CLI 命令（建议）与职责说明

```
bcindex init --root /path/repo
bcindex index --root /path/repo --full
bcindex watch --root /path/repo
bcindex query --repo repo_name --q "SearchTerm" --type symbol
bcindex status --repo repo_name
```

说明：
- `init`：注册仓库与初始化元数据（repo_id、默认配置、索引目录结构）。
  - 为什么需要：索引系统需要把“路径”转成稳定的 `repo_id`，并生成 `.bcindex/repos/<repo_id>/` 目录；也方便后续多仓管理、权限控制与索引隔离。
  - 如果你不想单独暴露 `init`，可以在 `index --full` 中自动完成（首次执行时自动 init）。
- `--root` 未指定时：默认从当前工作目录向上查找最近的 Git 仓库根目录（`.git` 所在路径）。若找不到则报错并提示显式指定 `--root`。
- `index --full`：全量索引，适合首次构建或大版本重建。
- `index --diff <rev>`：增量索引（可选），只处理变更文件，后续可以补充该参数。
- `watch`：监听 Git 状态变化，触发增量索引。
- `query`：本地查询入口，复用与 MCP/Skill 相同的查询内核。
- `status`：输出索引状态与最后一次索引时间。

### 7.2 MCP / Skill 命令（建议简化版）

MCP 不需要很多命令，建议保留最小集合：
- `repo.list`：列出已注册仓库与状态
- `search`：统一查询入口（按 type 参数区分）

#### search 统一接口设计

```
search(query, repo, type, top_k)
```

`type` 可取值：
- `text`：关键词/路径/正则检索（文本倒排索引）
- `symbol`：符号检索（函数/类型/方法/常量等）
- `mixed`：融合排序（符号匹配 + 文本匹配 + 向量检索）

#### type 的意义
- `text`：适合精准匹配、文件/路径定位、grep 级别的检索。
- `symbol`：适合 API/函数级检索（比如“GetLiveProducts”）。
- `mixed`：适合 LLM 场景，综合召回 + 排序，提高答案相关性。

## 8. 性能评估（MVP 粗略估计）

估算指标（单机）：
- Go AST 解析吞吐：3k~10k LOC/s（保守估计）
- 文本索引吞吐：5~20MB/s（依赖索引实现）
- Markdown：线性处理，通常远小于 Go 代码成本

估算公式：
```
总耗时 ≈ Go解析耗时 + 文本索引耗时 + IO
Go解析耗时 ≈ 总LOC / 解析吞吐
```

示例：
- 仓库 200k LOC：解析 20~60s，索引 5~15s，总体 30~90s

优化手段：
- 仅索引变更文件（增量索引）
- 缓存 AST 与 embedding 结果
- 并行解析（多 worker）

## 9. 安全与隔离

- 默认仅对用户可见（本地路径 + 用户级目录）。
- 可为 MCP/Skill 增加白名单仓库配置。
- 向量服务（如 OpenAI）需可配置开关，避免敏感代码外发。

## 10. 里程碑规划

- P0：Go/Markdown 全量索引 + CLI 查询
- P1：Git diff 增量索引 + watch
- P2：向量检索 + 混合排序
- P3：跨仓聚合查询与权限控制
