# BCIndex 下一版技术文档（VNext）

> 目标：在现有本地索引基础上，新增“关系图谱 + 索引分级 + 证据化输出”，为后续 agent 工具与影响分析打底。

## 1. 目标与范围

目标：
- 提升索引可解释性：每条结果可追溯“来源、证据、置信度”。
- 支持关系型检索：不仅返回“命中内容”，还能给出“相关联节点”。
- 引入索引分级：在速度、精度和依赖之间可配置权衡。
- 兼容现有实现：在不破坏现有 CLI 的前提下增量演进。

范围：
- 关系存储与最小关系抽取（imports、calls、depends_on）。
- 文档链接（Markdown 反引号符号 → 代码符号）。
- 可选的 gopls 增强解析（可失败降级）。
- 查询侧输出结构化上下文（保持 CLI 兼容）。

非目标：
- 分布式索引与远程协作。
- 全量数据流分析（仅做保守关系）。
- 多语言统一解析（先以 Go/Markdown 为核心）。

## 2. 设计原则

- **可降级**：外部工具不可用时退化到 fast 级别。
- **可解释**：每条关系与命中具备证据与置信度。
- **可迭代**：关系、解析、索引分级均可独立扩展。
- **可本地化**：默认零网络依赖，可选向量化。

## 3. 架构概览

```
CLI / MCP(后续)
        |
        v
Query Engine  ---> Result Composer(证据/关系上下文)
  |     |
Text  Symbol  Relation(optional)  Vector(optional)
  |     |          |
  +-----+----------+
        |
     Index Store (per repo)
        ^
        |
     Indexer Pipeline
        |
  File Collect -> AST Parse -> Project Link -> Enrich -> Persist
```

## 4. 索引分级（Index Tier）

为不同资源条件和准确度需求提供明确选择：

| Tier | 解析能力 | 关系能力 | 依赖 | 典型场景 |
| --- | --- | --- | --- | --- |
| fast | AST 符号 + 文本分块 | imports/depends_on（轻量） | 无 | 首次/快速索引 |
| balanced | fast + 文档链接 + 包级关系 | imports/calls/depends_on | `go list` | LLM 查询常用 |
| full | balanced + gopls 解析 | 更多调用/引用解析 | `gopls` | 影响分析/精确定位 |

行为约束：
- `full` 缺少 gopls 时 **fail-fast** 或自动降级（可配置）。
- `balanced` 保证无网络依赖。

## 5. 索引流水线（VNext）

1) 文件收集（git tracked + fallback）
2) AST 解析：符号提取 + 轻量关系提取
3) 项目级链接：包/模块关系、跨文件汇总
4) 文档链接：Markdown 反引号符号 → 符号索引
5) 可选 gopls 增强：解析调用/引用目标
6) 持久化：符号、关系、文档、文件元数据

## 6. 数据模型（SQLite 规划）

### 6.1 基础表

```
symbols(
  id INTEGER PRIMARY KEY,
  name TEXT,
  qualified_name TEXT,
  kind TEXT,
  file TEXT,
  line INTEGER,
  pkg TEXT,
  recv TEXT,
  doc TEXT,
  visibility TEXT
)

files(
  path TEXT PRIMARY KEY,
  hash TEXT,
  lang TEXT,
  size INTEGER,
  mtime INTEGER
)
```

### 6.2 关系表（最小集）

```
relations(
  id INTEGER PRIMARY KEY,
  from_symbol_id INTEGER,
  to_symbol_id INTEGER,
  to_qualified TEXT,
  kind TEXT,           -- imports/calls/depends_on/documents
  file TEXT,
  line INTEGER,
  confidence REAL,    -- 0.0 ~ 1.0
  source TEXT         -- ast/heuristic/gopls/doc
)
```

### 6.3 文档与链接

```
docs(
  id INTEGER PRIMARY KEY,
  path TEXT,
  title TEXT,
  level INTEGER,
  line_start INTEGER,
  line_end INTEGER,
  content_hash TEXT
)

doc_links(
  doc_id INTEGER,
  symbol_id INTEGER,
  kind TEXT,          -- documents/specifies
  file TEXT,
  line INTEGER,
  confidence REAL,
  source TEXT         -- md_backtick
)
```

## 7. 证据与置信度规范

每条关系或命中结果需携带最小证据字段：
- `source`：ast / heuristic / gopls / md_backtick
- `file` + `line`：定位证据来源
- `confidence`：0~1 的置信度（解析层定义）

用途：
- 查询排序与混排。
- 输出可解释性（“为何命中”）。
- 回溯与调试（索引质量问题排查）。

## 8. 查询与输出策略

### 8.1 查询类型（兼容现有）

- `text`：关键词/路径检索
- `symbol`：符号精确/前缀匹配
- `mixed`：符号 + 文本 + 关系补全（可选向量）

### 8.2 关系补全（VNext 新增）

- 对命中符号补充 `imports/calls/depends_on` 关系摘要。
- 文档命中时补充对应的符号链接。

### 8.3 输出预算控制

- 按 `query_top_k` 与 `max_context_chars` 控制输出体积。
- 关系补全按 `max_relation_per_hit` 截断。

## 9. 配置草案（增量字段）

```
index:
  tier: fast | balanced | full
  lsp_timeout_secs: 600
  lsp_fail_fast: true
query:
  max_context_chars: 20000
  max_relation_per_hit: 8
```

## 10. 兼容与迁移

- 默认 `tier=fast`，不影响现有索引与查询。
- 数据库可增量升级：新增表、字段不破坏旧数据。
- 缺少 gopls 时自动降级或提示安装。
