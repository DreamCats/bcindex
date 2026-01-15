# CHANGELOG

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
