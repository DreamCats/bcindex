# BCIndex 下一版迭代清单（VNext）

> 范围：以“关系索引 + 证据输出 + 索引分级”为核心，兼容现有 CLI 行为。

## 1. 里程碑

N1 - 索引分级与关系存储  
N2 - 文档链接与证据化输出  
N3 - 查询补全与影响分析雏形  
N4 - 稳定性与测试  

## 2. 任务清单

### N1 - 索引分级与关系存储

1) 配置与参数
- 新增 `index.tier` 配置与 CLI 参数（fast/balanced/full）。
- tier 行为矩阵与默认降级策略。

2) 关系存储
- 新增 SQLite `relations` 表结构与写入接口。
- AST 阶段抽取 `imports` 与包级 `depends_on` 关系。

3) 项目级链接
- 基于 `go list` 构建包依赖关系（balanced 起）。
- 索引元数据落库，支持增量对比。

### N2 - 文档链接与证据化输出

1) Markdown 反引号符号链接
- 解析 `docs` 块并提取反引号符号。
- 建立 `doc_links` 并记录 `source=md_backtick`。

2) 证据字段
- 对符号/关系记录 `source` 与 `confidence`。
- 输出时保留 `file`/`line` 证据。

### N3 - 查询补全与影响分析雏形

1) 关系补全
- `mixed` 查询补充 `imports/calls/depends_on` 摘要。
- 新增 `max_relation_per_hit` 限制。

2) 影响分析（最小形态）
- 新增 `bcindex impact --symbol <name>`，输出依赖链摘要。
- 结果包含来源与置信度。

3) 输出预算控制
- 新增 `query.max_context_chars`。
- 对超限结果进行截断提示。

### N4 - 稳定性与测试

1) gopls 可选集成
- `full` 模式接入 gopls，缺失时 fail-fast 或降级。
- `lsp_timeout_secs` 支持超时配置。

2) 测试与质量
- 关系抽取单测（imports/depends_on）。
- 文档链接单测（反引号符号）。
- `impact` 查询集成测试。

## 3. 交付物

- `reference/BCINDEX_VNEXT_TECH_SPEC.md` 技术文档更新。
- SQLite 关系存储 + 查询补全能力。
- CLI 新增 `impact` 与相关配置项。
