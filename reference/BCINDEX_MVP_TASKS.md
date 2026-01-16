# BCIndex MVP 任务拆解（Go）

> 范围：本地索引 + CLI（不包含 MCP stdio），按仓库独立存储。

## 1. 里程碑

M1 - 核心索引流水线  
M2 - CLI 命令集  
M3 - 查询引擎与返回结果  
M4 - 基础测试与打包  

## 2. 任务清单

### M1 - 核心索引流水线

1) 仓库注册与元数据
- 定义 repo_id（绝对路径哈希）。
- 实现 `repo.json` 与索引目录结构。
- `init` 与 `index --full` 自动初始化逻辑。

2) 文件发现
- `git ls-files` 获取跟踪文件。
- 过滤扩展名：`.go`, `.md`, `.markdown`。

3) Go 符号抽取
- AST walk 提取：
  - functions, methods, types, interfaces, const, var
  - receiver 名称与类型
  - doc 注释摘要
- 写入 SQLite `symbols` 表。

4) Markdown 分块
- 解析 `#`..`####` 标题层级。
- 构建标题路径 + 行号范围的 chunk。
- 写入文本索引（向量索引后续可加）。

5) 文本索引写入
- 设计 `TextIndexer` 接口（add/update/remove）。
- MVP 选型：SQLite FTS5 或 Bleve（二选一）。

### M2 - CLI 命令

1) 命令框架
- 使用 `cobra` 或 `urfave/cli`。
- 实现 `init`, `index`, `query`, `status`。

2) Root 自动识别
- `--root` 为空时向上查找 `.git`。
- 找不到时报错并提示。

3) 状态输出
- 输出上次索引时间、文件数、索引大小。

### M3 - 查询引擎

1) Query Router
- `type`: `text` / `symbol` / `mixed`。

2) Symbol Search
- `symbols.name` 精确或前缀匹配。
- 模糊匹配后续再加。

3) Text Search
- 关键词 / 路径检索。

4) Mixed Ranking
- 合并文本与符号结果，权重排序。
- 返回 top_k，附带文件与行号上下文。

### M4 - 测试与打包

1) 单元测试
- Go AST 解析样例。
- Markdown 分块正确性。
- SQLite 写入与查询。

2) 集成测试
- 索引小仓库样例。
- 查询已知符号与关键词。

3) 打包
- `go build` 生成 release。
- README 简要安装说明。

## 3. 周期估算（MVP）

- M1：2-3 周
- M2：1 周
- M3：1-2 周
- M4：3-5 天

总计：1-2 人 4-6 周。

## 4. 风险与缓解

- AST 边界情况：先覆盖主流语法，解析失败跳过并记录日志。
- 文本索引膨胀：按仓库隔离，控制字段规模。
- 混合排序效果不足：先用简单权重，后续引入 rerank。
- 后续增量/监听：MVP 仅全量索引，后续增加 `watch` 与 diff 增量。
