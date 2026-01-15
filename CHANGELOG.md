# CHANGELOG

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
