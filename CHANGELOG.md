# CHANGELOG

## 2026-01-15

- 版本：0.3.10
- 变更：新增本地索引与检索技术方案文档，补充架构、流程与参数说明。
- 影响文件：`reference/BCINDEX_TECH_SOLUTION.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：无

## 2025-01-15

- 版本：0.3.12
- 变更：mixed 查询增加“先过滤再向量”策略，本地向量检索按候选集 rerank（可配置 `vector_rerank_candidates`）。
- 影响文件：`internal/bcindex/query.go`、`internal/bcindex/vector_store.go`、`internal/bcindex/vector_config.go`、`README.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.3.11
- 变更：索引时自动生成默认向量配置，并在缺少 API Key/Model 时友好提示并降级。
- 影响文件：`internal/bcindex/indexer.go`、`internal/bcindex/cli.go`、`README.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.3.10
- 变更：`index --diff` 在索引缺失时自动回退到全量索引，减少首次使用负担；补充 README 提示。
- 影响文件：`internal/bcindex/cli.go`、`internal/bcindex/index_check.go`、`README.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.3.9
- 变更：新增向量查询与 mixed 融合（text/symbol/vector），输出补充 source 字段。
- 影响文件：`internal/bcindex/query.go`、`internal/bcindex/vector_store.go`、`internal/bcindex/qdrant_client.go`、`internal/bcindex/types.go`、`internal/bcindex/cli.go`、`README.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.3.8
- 变更：索引完成提示包含 text/symbol/vector 阶段信息（含 diff 场景）。
- 影响文件：`internal/bcindex/cli.go`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.3.7
- 变更：索引阶段增加 phase 提示（text/symbol、vector 模式与等待阶段）。
- 影响文件：`internal/bcindex/indexer.go`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.3.6
- 变更：本地向量库写入加锁与 busy_timeout，避免 SQLITE_BUSY；向量错误增加 `vector:` 前缀便于识别。
- 影响文件：`internal/bcindex/vector_store.go`、`internal/bcindex/indexer.go`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.3.5
- 变更：Volces embedding 响应兼容 data 为对象/数组的格式，避免解析失败。
- 影响文件：`internal/bcindex/volces_embeddings.go`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.3.4
- 变更：向量存储改为本地模式（`qdrant_path` 走本地 SQLite 存储，不依赖 Qdrant 进程），新增本地向量存储实现与配置说明。
- 影响文件：`internal/bcindex/vector_store.go`、`internal/bcindex/vector_runtime.go`、`internal/bcindex/indexer.go`、`internal/bcindex/vector_config.go`、`README.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.3.3
- 变更：自动下载 Qdrant 二进制（无 qdrant_bin 且 PATH 中缺失时），与 docs-hub 使用体验对齐。
- 影响文件：`internal/bcindex/qdrant_download.go`、`internal/bcindex/qdrant_process.go`、`README.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：无

## 2025-01-15

- 版本：0.3.2
- 变更：`config init` 默认写入 `qdrant_path=~/.bcindex/qdrant`，输出最小配置，补充 README 配置示例。
- 影响文件：`internal/bcindex/vector_config.go`、`README.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：无

## 2025-01-15

- 版本：0.3.1
- 变更：向量索引 Phase 2（Go 函数级分块、全量清理旧向量、增量更新复用向量运行时、并发 worker + 批量 embedding），新增 `vector_workers` 配置。
- 影响文件：`internal/bcindex/indexer.go`、`internal/bcindex/vector_chunks.go`、`internal/bcindex/vector_config.go`、`internal/bcindex/vector_runtime.go`、`internal/bcindex/qdrant_client.go`、`README.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.3.0
- 变更：新增向量分块与写入/删除流程，支持 file->vector_ids 映射。
- 影响文件：`internal/bcindex/indexer.go`、`internal/bcindex/vector_chunks.go`、`internal/bcindex/vector_types.go`、`internal/bcindex/symbol_store.go`、`internal/bcindex/vector_config.go`、`README.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.2.3
- 变更：配置示例改为 docs-hub 风格（以 qdrant_path 为主，附可选字段说明）。
- 影响文件：`README.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：无

## 2025-01-15

- 版本：0.2.2
- 变更：支持 `qdrant_path` 自动启动 Qdrant 进程（本地存储模式）。
- 影响文件：`internal/bcindex/qdrant_process.go`、`internal/bcindex/qdrant_client.go`、`internal/bcindex/vector_config.go`、`README.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.2.1
- 变更：新增 `config init` 命令，生成默认向量配置文件。
- 影响文件：`internal/bcindex/cli.go`、`internal/bcindex/vector_config.go`、`README.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.2.0
- 变更：向量化配置改为用户级配置文件（`~/.bcindex/config/bcindex.yaml`）。
- 影响文件：`internal/bcindex/vector_config.go`、`README.md`、`PROJECT_META.md`、`CHANGELOG.md`、`go.mod`、`go.sum`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.1.9
- 变更：新增 Qdrant 与 Volces embedding 客户端及配置入口（Phase 1）。
- 影响文件：`internal/bcindex/qdrant_client.go`、`internal/bcindex/volces_embeddings.go`、`internal/bcindex/vector_config.go`、`internal/bcindex/vector_types.go`、`README.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.1.8
- 变更：补充分阶段实现计划（Phase 1-4）到向量技术文档。
- 影响文件：`reference/BCINDEX_VECTOR_TECH_SPEC.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：无

## 2025-01-15

- 版本：0.1.7
- 变更：向量化策略改为默认开启并可显式关闭，补充索引并发建议与参数。
- 影响文件：`reference/BCINDEX_VECTOR_TECH_SPEC.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：无

## 2025-01-15

- 版本：0.1.6
- 变更：新增向量化与混合检索技术文档，覆盖设计原因与参数策略。
- 影响文件：`reference/BCINDEX_VECTOR_TECH_SPEC.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：无

## 2025-01-15

- 版本：0.1.5
- 变更：新增 `version` 命令，读取 `PROJECT_META.md` 输出版本号。
- 影响文件：`internal/bcindex/cli.go`、`internal/bcindex/version.go`、`README.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.1.4
- 变更：watch 增加去抖与批处理，索引过程改为单文件失败可继续，并补充按 path 清理旧文档的兜底逻辑。
- 影响文件：`internal/bcindex/cli.go`、`internal/bcindex/indexer.go`、`internal/bcindex/diff.go`、`internal/bcindex/symbol_store.go`、`README.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.1.3
- 变更：新增增量索引（基于 git diff）与 watch 监听模式。
- 影响文件：`internal/bcindex/indexer.go`、`internal/bcindex/diff.go`、`internal/bcindex/symbol_store.go`、`internal/bcindex/cli.go`、`README.md`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.1.2
- 变更：新增索引进度条与查询中 spinner，默认在终端显示。
- 影响文件：`internal/bcindex/indexer.go`、`internal/bcindex/progress.go`、`internal/bcindex/cli.go`、`README.md`、`go.mod`、`go.sum`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.1.1
- 变更：text 搜索增加标题/路径权重（查询侧 boost），mixed 去重与排序优化（符号优先）。
- 影响文件：`internal/bcindex/query.go`、`internal/bcindex/text_index.go`、`PROJECT_META.md`、`CHANGELOG.md`。
- 结果：`go build ./cmd/bcindex`

## 2025-01-15

- 版本：0.1.0
- 变更：新增项目元信息文件，明确版本号记录方式。
- 影响文件：`PROJECT_META.md`、`AGENTS.md`、`CHANGELOG.md`。
- 结果：无

## 2025-01-15

- 版本：0.1.0
- 变更：查询支持 JSON 输出，并在未索引时给出明确指引。
- 影响文件：`internal/bcindex/cli.go`、`internal/bcindex/query.go`、`internal/bcindex/repo.go`、`internal/bcindex/status.go`、`internal/bcindex/types.go`、`internal/bcindex/index_check.go`、`README.md`。
- 结果：`go build ./cmd/bcindex`
